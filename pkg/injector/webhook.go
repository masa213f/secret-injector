package injector

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// InjectClient injects a client.
func (in *Injector) InjectClient(client client.Client) error {
	in.client = client
	return nil
}

type annotation struct {
	AutoPrune bool
	Owner     string
	Repo      string
	Source    string
	Branch    string
	FileHash  *string
	DirHash   map[string]string
}

func (in *Injector) decodeAnnotation(sec *corev1.Secret) (*annotation, error) {
	autoprune := sec.Annotations[AutoPruneKey]

	val, exist := sec.Annotations[RepositoryKey]
	if !exist {
		return nil, errors.New("no annotation :" + RepositoryKey)
	}
	repo := strings.Split(val, "/")
	if len(repo) != 2 || repo[0] == "" || repo[1] == "" {
		return nil, errors.New("invalid annotation: " + RepositoryKey)
	}
	source, exist := sec.Annotations[SourceKey]
	if !exist {
		return nil, errors.New("no annotation :" + SourceKey)
	}
	branch, exist := sec.Annotations[BranchKey]
	if !exist {
		branch = ""
	}

	var fileHash *string
	val, exist = sec.Annotations[HashKey]
	if exist {
		fileHash = &val
	}

	dirHash := map[string]string{}
	for k, v := range sec.Annotations {
		if !strings.HasPrefix(k, HashKeyPrefix) {
			continue
		}
		name := strings.TrimPrefix(k, HashKeyPrefix)
		dirHash[name] = v
	}

	ret := annotation{
		AutoPrune: autoprune == "true",
		Owner:     repo[0],
		Repo:      repo[1],
		Source:    source,
		Branch:    branch,
		FileHash:  fileHash,
		DirHash:   dirHash,
	}
	return &ret, nil
}

// Handle handles addmission requests.
func (in *Injector) Handle(ctx context.Context, req admission.Request) admission.Response {
	log := in.log.WithName("webhook")

	sec := &corev1.Secret{}
	err := in.decoder.Decode(req, sec)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if sec.Labels[WebhookTargetKey] != WebhookTargetValue {
		return admission.Allowed("ok")
	}

	log.Info("Mutating Secrets", "namespace", req.Namespace, "name", req.Name)

	annotation, err := in.decodeAnnotation(sec)
	if err != nil {
		log.Error(err, "Could not decode annotations")
		return admission.Errored(http.StatusInternalServerError, err)
	}

	source, err := in.fetch(annotation.Owner, annotation.Repo, annotation.Source, annotation.Branch)
	if err != nil {
		log.Error(err, "Could not fetch source")
		return admission.Errored(http.StatusInternalServerError, err)
	}

	if sec.Data == nil || annotation.AutoPrune {
		sec.Data = map[string][]byte{}
	}

	// Remove unnecessary hash annotations
	delete(sec.Annotations, HashKey)
	for k := range sec.Annotations {
		if strings.HasPrefix(k, HashKeyPrefix) {
			delete(sec.Annotations, k)
		}
	}

	if source.Type == typeFile &&
		(annotation.FileHash == nil || *annotation.FileHash != source.Meta[0].Hash) {
		sec.Annotations[HashKey] = source.Meta[0].Hash
		for k, v := range source.Data {
			sec.Data[k] = []byte(v)
		}
	} else if source.Type == typeDir {
		for _, meta := range source.Meta {
			if annotation.DirHash[meta.Name] == meta.Hash {
				continue
			}
			sec.Annotations[HashKeyPrefix+meta.Name] = meta.Hash
			sec.Data[meta.Name] = []byte(source.Data[meta.Name])
		}
	}

	marshaled, err := json.Marshal(sec)
	if err != nil {
		log.Error(err, "Could not marshal secret")
		return admission.Errored(http.StatusInternalServerError, err)
	}

	log.Info("Success Mutating Secrets", "namespace", req.Namespace, "name", req.Name)
	return admission.PatchResponseFromRaw(req.Object.Raw, marshaled)
}
