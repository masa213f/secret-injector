package injector

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/go-github/v30/github"
	"golang.org/x/oauth2"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// Injector is mutateing webhook and controller.
type Injector struct {
	client          client.Client
	decoder         *admission.Decoder
	githubClient    *github.Client
	pollingInterval time.Duration
	log             logr.Logger
}

type sourceType int

const (
	typeFile = iota
	typeDir
)

type source struct {
	Type sourceType
	Meta []metadata
	Data map[string]string
}

type metadata struct {
	Name string
	Hash string
}

// New creates the new Injector.
func New(githubToken string, pollingInterval time.Duration, log logr.Logger) *Injector {
	var c *http.Client
	if githubToken != "" {
		ctx := context.Background()
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: githubToken},
		)
		c = oauth2.NewClient(ctx, ts)
	}

	return &Injector{
		githubClient:    github.NewClient(c),
		pollingInterval: pollingInterval,
		log:             log,
	}
}

// SetupWithManager sets up reconciler and webhook.
func (in *Injector) SetupWithManager(mgr manager.Manager) error {
	// setup webhook
	hookServer := mgr.GetWebhookServer()
	hookServer.Register("/secrets/mutate", &admission.Webhook{Handler: in})

	// setup controller
	if err := corev1.AddToScheme(mgr.GetScheme()); err != nil {
		return err
	}
	if _, err := mgr.GetCache().GetInformer(&corev1.Secret{}); err != nil {
		return err
	}
	return builder.ControllerManagedBy(mgr).
		WithEventFilter(predicate.Funcs{
			CreateFunc:  func(event.CreateEvent) bool { return true },
			DeleteFunc:  func(event.DeleteEvent) bool { return false },
			UpdateFunc:  func(event.UpdateEvent) bool { return true },
			GenericFunc: func(event.GenericEvent) bool { return false },
		}).
		For(&corev1.Secret{}).
		Complete(in)
}

// InjectDecoder injects a decoder.
func (in *Injector) InjectDecoder(d *admission.Decoder) error {
	in.decoder = d
	return nil
}

func (in *Injector) calcWaitTime(sec *corev1.Secret, now time.Time) time.Duration {
	val, exist := sec.Annotations[LastUpdateKey]
	if !exist {
		return 0
	}
	prevTime, err := time.Parse(time.RFC3339, val)
	if err != nil {
		return 0
	}
	wait := prevTime.Add(in.pollingInterval).Sub(now)
	if wait < time.Second {
		return 0
	}
	return wait
}

func (in *Injector) fetch(owner, repo, path, branch string) (*source, error) {
	fileContent, dirContent, _, err := in.githubClient.Repositories.GetContents(
		context.Background(), owner, repo, path, &github.RepositoryContentGetOptions{Ref: branch})
	if _, ok := err.(*github.RateLimitError); ok {
		return nil, errors.New("RateLimitError")
	} else if err != nil {
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
			Type: typeFile,
			Meta: []metadata{
				{Name: fileContent.GetName(), Hash: fileContent.GetSHA()},
			},
			Data: data,
		}
		return &ret, nil
	}

	meta := []metadata{}
	data := map[string]string{}
	for _, fileMeta := range dirContent {
		if fileMeta.Type != nil && *fileMeta.Type != "file" {
			continue
		}

		fileData, _, _, err := in.githubClient.Repositories.GetContents(
			context.Background(), owner, repo, fileMeta.GetPath(), &github.RepositoryContentGetOptions{Ref: branch})
		if _, ok := err.(*github.RateLimitError); ok {
			return nil, errors.New("RateLimitError")
		} else if err != nil {
			return nil, err
		}
		raw, err := fileData.GetContent()
		if err != nil {
			return nil, err
		}
		meta = append(meta, metadata{Name: fileMeta.GetName(), Hash: fileMeta.GetSHA()})
		data[fileMeta.GetName()] = raw
	}

	ret := source{
		Type: typeDir,
		Meta: meta,
		Data: data,
	}
	return &ret, nil
}

func (in *Injector) isTarget(sec *corev1.Secret) bool {
	return false
}

// Reconcile rewrite secrets data.
func (in *Injector) Reconcile(req reconcile.Request) (reconcile.Result, error) {
	sec := &corev1.Secret{}
	err := in.client.Get(context.TODO(), req.NamespacedName, sec)
	if apierrors.IsNotFound(err) {
		in.log.Error(nil, "Could not find Secret")
		return reconcile.Result{}, nil
	} else if err != nil {
		in.log.Error(err, "Could not fetch Secret")
		return reconcile.Result{}, err
	}

	if !in.isTarget(sec) {
		return reconcile.Result{}, nil
	}
	log := in.log.WithName("controller")
	now := time.Now()
	if wait := in.calcWaitTime(sec, now); wait > 0 {
		log.Info("Reconciling skip", "namespace", req.NamespacedName.Namespace, "name", req.NamespacedName.Name)
		return reconcile.Result{RequeueAfter: wait}, nil
	}
	log.Info("Reconciling Secrets", "namespace", req.NamespacedName.Namespace, "name", req.NamespacedName.Name)

	// err = in.update(sec, now)
	// if err != nil {
	// 	log.Error(err, "Could not update Secret")
	// 	return reconcile.Result{}, err
	// }

	// err = in.client.Update(context.TODO(), sec)
	// if err != nil {
	// 	log.Error(err, "Could not write Secret")
	// 	return reconcile.Result{}, err
	// }

	log.Info("Success Reconciling Secrets", "namespace", req.NamespacedName.Namespace, "name", req.NamespacedName.Name)
	return reconcile.Result{}, nil
}
