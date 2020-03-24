package injector

import (
	"context"
	"encoding/json"
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

// New creates the new Injector.
func New(githubToken string, pollingInterval time.Duration, log logr.Logger) *Injector {
	var g *github.Client
	if githubToken == "" {
		g = github.NewClient(nil)
	} else {
		ctx := context.Background()
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: githubToken},
		)
		tc := oauth2.NewClient(ctx, ts)
		g = github.NewClient(tc)
	}

	return &Injector{
		githubClient:    g,
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

// InjectClient injects a client.
func (in *Injector) InjectClient(client client.Client) error {
	in.client = client
	return nil
}

// InjectDecoder injects a decoder.
func (in *Injector) InjectDecoder(d *admission.Decoder) error {
	in.decoder = d
	return nil
}

func (in *Injector) isTarget(sec *corev1.Secret) bool {
	val, exist := sec.Labels[TargetLabelKey]
	return exist && val == TargetLabelValue
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

func (in *Injector) fetch(owner, repo, path, branch string) (map[string]string, error) {
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

		ret := map[string]string{}
		for k, v := range data {
			ret[k] = v
		}
		return ret, nil
	}

	ret := map[string]string{}
	for _, fileMeta := range dirContent {
		if fileMeta.Type != nil && *fileMeta.Type != "file" {
			continue
		}

		file, _, _, err := in.githubClient.Repositories.GetContents(
			context.Background(), owner, repo, fileMeta.GetPath(), &github.RepositoryContentGetOptions{Ref: branch})
		if _, ok := err.(*github.RateLimitError); ok {
			return nil, errors.New("RateLimitError")
		} else if err != nil {
			return nil, err
		}

		data, err := file.GetContent()
		if err != nil {
			return nil, err
		}
		ret[file.GetName()] = data
	}
	return ret, nil
}

func (in *Injector) update(sec *corev1.Secret, now time.Time) error {
	sec.Annotations[LastUpdateKey] = now.UTC().Format(time.RFC3339)

	data, err := in.fetch("masa213f", "secret-injector", "testdata/files", "")
	if err != nil {
		return err
	}
	for k, v := range data {
		sec.Data[k] = []byte(v)
	}
	return nil
}

// Handle handles addmission requests.
func (in *Injector) Handle(ctx context.Context, req admission.Request) admission.Response {
	sec := &corev1.Secret{}
	err := in.decoder.Decode(req, sec)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if !in.isTarget(sec) {
		return admission.Allowed("ok")
	}
	now := time.Now()
	if in.calcWaitTime(sec, now) > 0 {
		return admission.Allowed("ok")
	}

	log := in.log.WithName("webhook")
	log.Info("Mutating Secrets", "namespace", req.Namespace, "name", req.Name)

	err = in.update(sec, now)
	if err != nil {
		log.Error(err, "Could not update Secret")
		return admission.Errored(http.StatusInternalServerError, err)
	}

	marshaled, err := json.Marshal(sec)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	log.Info("Success Mutating Secrets", "namespace", req.Namespace, "name", req.Name)
	return admission.PatchResponseFromRaw(req.Object.Raw, marshaled)
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

	err = in.update(sec, now)
	if err != nil {
		log.Error(err, "Could not update Secret")
		return reconcile.Result{}, err
	}

	err = in.client.Update(context.TODO(), sec)
	if err != nil {
		log.Error(err, "Could not write Secret")
		return reconcile.Result{}, err
	}

	log.Info("Success Reconciling Secrets", "namespace", req.NamespacedName.Namespace, "name", req.NamespacedName.Name)
	return reconcile.Result{}, nil
}
