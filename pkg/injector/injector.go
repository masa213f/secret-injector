package injector

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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
	pollingInterval time.Duration
	log             logr.Logger
}

// New creates the new Injector.
func New(pollingInterval time.Duration, log logr.Logger) *Injector {
	return &Injector{
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
	val, exist := sec.Annotations[AnnotationKey]
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

// GetData is xxx
func (in *Injector) getData() string {
	const len = 64
	runes := make([]byte, len)

	for i := 0; i < len; i++ {
		num, _ := rand.Int(rand.Reader, big.NewInt(255))
		runes[i] = byte(num.Int64())
	}

	return base64.RawStdEncoding.EncodeToString(runes)
}

func (in *Injector) update(sec *corev1.Secret, now time.Time) error {
	sec.Annotations[AnnotationKey] = now.UTC().Format(time.RFC3339)

	for k := range sec.Data {
		sec.Data[k] = []byte(in.getData())
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
	if errors.IsNotFound(err) {
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
