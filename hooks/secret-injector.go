package hooks

import (
	"context"
	"encoding/json"
	"fmt"
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

	_, ok := sec.Labels["secret-injector.m213f.org/injection"]
	if !ok {
		return admission.Allowed("ok")
	}

	if sec.Type != corev1.SecretTypeOpaque {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("type is not Opaqueu"))
	}
	if sec.Data == nil {
		sec.Data = map[string][]byte{}
	}
	sec.Data["hoge"] = []byte("hogehogehoge")
	sec.Data["piyo"] = []byte("piyopiyopiyo")
	sec.Data["fuga"] = []byte("fugafugafuga")

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
