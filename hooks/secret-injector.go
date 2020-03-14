package hooks

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type secretInjector struct {
	client      client.Client
	decoder     *admission.Decoder
	githubToken string
}

func newSecret() string {
	runes := make([]byte, 64)

	for i := 0; i < 64; i++ {
		num, _ := rand.Int(rand.Reader, big.NewInt(255))
		runes[i] = byte(num.Int64())
	}

	return base64.RawStdEncoding.EncodeToString(runes)
}

// +kubebuilder:webhook:path=/secrets/mutate,mutating=true,failurePolicy=fail,groups="",resources=secrets,verbs=create;update,versions=v1,name=secret-injector.m213f.org

// NewSecretInjector creates a webhook handler for Secret.
func NewSecretInjector(c client.Client, githubToken string) http.Handler {
	return &webhook.Admission{Handler: &secretInjector{client: c, githubToken: githubToken}}
}

func (s *secretInjector) Handle(ctx context.Context, req admission.Request) admission.Response {
	sec := &corev1.Secret{}
	err := s.decoder.Decode(req, sec)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	val, ok := sec.Labels["secret-injector.m213f.org/injection"]
	if !ok || val != "true" {
		return admission.Allowed("ok")
	}

	if sec.Type != corev1.SecretTypeOpaque {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("type is not Opaqueu"))
	}
	for k, _ := range sec.Data {
		sec.Data[k] = []byte(newSecret())
	}

	marshaled, err := json.Marshal(sec)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return admission.PatchResponseFromRaw(req.Object.Raw, marshaled)
}

// InjectDecoder injects the decoder.
func (s *secretInjector) InjectDecoder(d *admission.Decoder) error {
	s.decoder = d
	return nil
}
