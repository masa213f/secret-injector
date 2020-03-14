package secretinjector

import (
	"context"
	"encoding/json"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// Handle handles addmission requests.
func (si *SecretInjector) Handle(ctx context.Context, req admission.Request) admission.Response {
	sec := &corev1.Secret{}
	err := si.decoder.Decode(req, sec)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if !isTargetSecret(sec) {
		return admission.Allowed("ok")
	}

	si.log.Info("Mutating Secrets", "namespace", req.Namespace, "name", req.Name)

	for k := range sec.Data {
		sec.Data[k] = []byte(newSecret())
	}

	marshaled, err := json.Marshal(sec)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, marshaled)
}

// InjectDecoder injects the decoder.
func (si *SecretInjector) InjectDecoder(d *admission.Decoder) error {
	si.decoder = d
	return nil
}
