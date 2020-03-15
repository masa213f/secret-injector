package secretinjector

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// Handle handles addmission requests.
func (si *SecretInjector) Handle(ctx context.Context, req admission.Request) admission.Response {
	log := si.log.WithName("webhook")

	sec := &corev1.Secret{}
	err := si.decoder.Decode(req, sec)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if !isTargetSecret(sec) {
		return admission.Allowed("ok")
	}

	log.Info("Mutating Secrets", "namespace", req.Namespace, "name", req.Name)

	if sec.Annotations == nil {
		sec.Annotations = map[string]string{}
	}
	sec.Annotations[AnnotationKey] = sec.CreationTimestamp.UTC().Format(time.RFC3339)

	for k := range sec.Data {
		sec.Data[k] = []byte(newSecret())
	}

	marshaled, err := json.Marshal(sec)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, marshaled)
}

// InjectDecoder injects a decoder.
func (si *SecretInjector) InjectDecoder(d *admission.Decoder) error {
	si.decoder = d
	return nil
}
