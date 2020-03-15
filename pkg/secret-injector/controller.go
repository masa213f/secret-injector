package secretinjector

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Implement reconcile.Reconciler so the controller can reconcile objects
var _ reconcile.Reconciler = &SecretInjector{}

func (si *SecretInjector) needUpdate(sec *corev1.Secret, now time.Time) bool {
	if sec == nil {
		return false
	}
	log := si.log.WithName("controller")

	val, ok := sec.Annotations[AnnotationKey]
	if !ok {
		log.Error(nil, "Does not set update timestamp")
		return true
	}
	prevTime, err := time.Parse(time.RFC3339, val)
	if err != nil {
		log.Error(err, "Could not parse update timestamp")
		return true
	}
	return prevTime.Add(5 * time.Second).Before(now)
}

// Reconcile rewrite secrets data.
func (si *SecretInjector) Reconcile(req reconcile.Request) (reconcile.Result, error) {
	log := si.log.WithName("controller")

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

	now := time.Now()
	if !si.needUpdate(sec, now) {
		log.Info("Reconciling skip", "secret name", req.NamespacedName)
		return reconcile.Result{Requeue: true}, nil
	}

	if sec.Annotations == nil {
		sec.Annotations = map[string]string{}
	}
	sec.Annotations[AnnotationKey] = now.UTC().Format(time.RFC3339)
	for k := range sec.Data {
		sec.Data[k] = []byte(newSecret())
	}

	err = si.client.Update(context.TODO(), sec)
	if err != nil {
		log.Error(err, "Could not write Secret")
		return reconcile.Result{}, err
	}
	return reconcile.Result{Requeue: true}, nil
}

// InjectClient injects a client.
func (si *SecretInjector) InjectClient(c client.Client) error {
	si.client = c
	return nil
}
