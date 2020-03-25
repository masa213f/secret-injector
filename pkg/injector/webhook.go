package injector

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-logr/logr"
	"github.com/google/go-github/v30/github"
	"golang.org/x/oauth2"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// Injector is mutateing webhook and controller.
type Injector struct {
	decoder      *admission.Decoder
	githubClient *github.Client
	log          logr.Logger
}

type option struct {
	owner  string
	repo   string
	branch string
	source string
	prune  bool
}

const (
	typeFile = iota
	typeDir
)

type source struct {
	srcType  int
	fileHash string
	dirHash  map[string]string
	data     map[string]string
}

// New creates the new Injector.
func New(githubToken string, log logr.Logger) admission.Handler {
	var c *http.Client
	if githubToken != "" {
		ctx := context.Background()
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: githubToken},
		)
		c = oauth2.NewClient(ctx, ts)
	}
	return &Injector{
		githubClient: github.NewClient(c),
		log:          log.WithName("webhook"),
	}
}

func (in *Injector) decodeAnnotations(sec *corev1.Secret) (*option, error) {
	val, exist := sec.Annotations[RepoNameKey]
	if !exist {
		return nil, errors.New("no annotation: " + RepoNameKey)
	}
	ownerRepo := strings.Split(val, "/")
	if len(ownerRepo) != 2 || ownerRepo[0] == "" || ownerRepo[1] == "" {
		return nil, errors.New("invalid annotation: " + RepoNameKey)
	}
	source, exist := sec.Annotations[SourcePathKey]
	if !exist {
		return nil, errors.New("no annotation: " + SourcePathKey)
	}
	branch := sec.Annotations[BranchNameKey]
	prune := sec.Annotations[PruneFlagKey]

	opt := option{
		owner:  ownerRepo[0],
		repo:   ownerRepo[1],
		branch: branch,
		source: source,
		prune:  prune == "true",
	}
	return &opt, nil
}

func (in *Injector) fetchSource(owner, repo, path, branch string) (*source, error) {
	fileContent, dirContent, _, err := in.githubClient.Repositories.GetContents(
		context.Background(), owner, repo, path, &github.RepositoryContentGetOptions{Ref: branch})
	if err != nil {
		return nil, err
	}

	if fileContent != nil {
		raw, err := fileContent.GetContent()
		if err != nil {
			return nil, err
		}
		data := map[string]string{}
		err = yaml.Unmarshal([]byte(raw), &data)
		if err != nil {
			return nil, err
		}

		ret := source{
			srcType:  typeFile,
			fileHash: fileContent.GetSHA(),
			data:     data,
		}
		return &ret, nil
	}

	hash := map[string]string{}
	data := map[string]string{}
	for _, fileMeta := range dirContent {
		if !(fileMeta.Type == nil || *fileMeta.Type == "file") {
			continue
		}

		fileData, _, _, err := in.githubClient.Repositories.GetContents(
			context.Background(), owner, repo, fileMeta.GetPath(), &github.RepositoryContentGetOptions{Ref: branch})
		if err != nil {
			return nil, err
		}
		str, err := fileData.GetContent()
		if err != nil {
			return nil, err
		}
		hash[fileMeta.GetName()] = fileMeta.GetSHA()
		data[fileMeta.GetName()] = str
	}

	ret := source{
		srcType: typeDir,
		dirHash: hash,
		data:    data,
	}
	return &ret, nil
}

// Handle handles addmission requests.
func (in *Injector) Handle(ctx context.Context, req admission.Request) admission.Response {
	sec := &corev1.Secret{}
	err := in.decoder.Decode(req, sec)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if sec.Labels[WebhookTargetKey] != "true" {
		return admission.Allowed("ok")
	}

	in.log.Info("Mutating Secrets", "namespace", req.Namespace, "name", req.Name)

	opt, err := in.decodeAnnotations(sec)
	if err != nil {
		in.log.Error(err, "Could not decode annotations")
		return admission.Errored(http.StatusBadRequest, err)
	}

	src, err := in.fetchSource(opt.owner, opt.repo, opt.source, opt.branch)
	if err != nil {
		in.log.Error(err, "Could not fetch source")
		return admission.Errored(http.StatusInternalServerError, err)
	}

	if sec.Data == nil || opt.prune {
		sec.Data = map[string][]byte{}
	}

	// Remove old hash annotations
	delete(sec.Annotations, SourceHashKey)
	for k := range sec.Annotations {
		if strings.HasPrefix(k, SourceHashKeyPrefix) {
			delete(sec.Annotations, k)
		}
	}

	// Update data
	if src.srcType == typeFile {
		sec.Annotations[SourceHashKey] = src.fileHash
		for k, v := range src.data {
			sec.Data[k] = []byte(v)
		}
	} else if src.srcType == typeDir {
		for name, hash := range src.dirHash {
			sec.Annotations[SourceHashKeyPrefix+name] = hash
			sec.Data[name] = []byte(src.data[name])
		}
	}

	marshaled, err := json.Marshal(sec)
	if err != nil {
		in.log.Error(err, "Could not marshal secret")
		return admission.Errored(http.StatusInternalServerError, err)
	}

	in.log.Info("Success Mutating Secrets", "namespace", req.Namespace, "name", req.Name)
	return admission.PatchResponseFromRaw(req.Object.Raw, marshaled)
}
