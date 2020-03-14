package secretinjector

import (
	"crypto/rand"
	"encoding/base64"
	"math/big"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// TargetLabelKey is the label key of target secrets.
const TargetLabelKey = "secret-injector.m213f.org/injection"

// TargetLabelValue is the label value of target secrets.
const TargetLabelValue = "true"

// SecretInjector is mutateing webhook and controller.
type SecretInjector struct {
	client  client.Client
	decoder *admission.Decoder
	log     logr.Logger
}

// New creates a controller for secrets.
func New(client client.Client, log logr.Logger) *SecretInjector {
	return &SecretInjector{client: client, log: log}
}

// SetupWithManager sets up Reconciler with Manager.
func (si *SecretInjector) SetupWithManager(mgr manager.Manager) error {
	// Setup webhook
	hookServer := mgr.GetWebhookServer()
	hookServer.Register("/secrets/mutate", &webhook.Admission{Handler: si})

	// Setup controller
	pred := predicate.Funcs{
		CreateFunc:  func(event.CreateEvent) bool { return false },
		DeleteFunc:  func(event.DeleteEvent) bool { return false },
		UpdateFunc:  func(event.UpdateEvent) bool { return true },
		GenericFunc: func(event.GenericEvent) bool { return false },
	}
	return builder.ControllerManagedBy(mgr).
		WithEventFilter(pred).
		For(&corev1.Secret{}).
		Complete(si)
}

func isTargetSecret(sec *corev1.Secret) bool {
	if sec == nil || sec.Labels == nil {
		return false
	}
	val, ok := sec.Labels[TargetLabelKey]
	return ok && val == TargetLabelValue
}

func newSecret() string {
	const len = 64
	runes := make([]byte, len)

	for i := 0; i < len; i++ {
		num, _ := rand.Int(rand.Reader, big.NewInt(255))
		runes[i] = byte(num.Int64())
	}

	return base64.RawStdEncoding.EncodeToString(runes)
}
