package secretinjector

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Reconcile rewrite secrets data.
func (si *SecretInjector) Reconcile(req reconcile.Request) (reconcile.Result, error) {
	log := si.log.WithValues("request", req)

	sec := &corev1.Secret{}
	err := si.client.Get(context.TODO(), req.NamespacedName, sec)
	if errors.IsNotFound(err) {
		log.Error(nil, "Could not find Secret")
		return reconcile.Result{}, nil
	} else if err != nil {
		log.Error(err, "Could not fetch Secret")
		return reconcile.Result{}, err
	}

	if !isTargetSecret(sec) {
		return reconcile.Result{}, nil
	}

	log.Info("Reconciling Secret", "secret name", req.NamespacedName)

	for k := range sec.Data {
		sec.Data[k] = []byte(newSecret())
	}

	err = si.client.Update(context.TODO(), sec)
	if err != nil {
		log.Error(err, "Could not write Secret")
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}
