/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	hazyv1alpha1 "github.com/jfgrea27/hazy-rabbit-operator/api/v1alpha1"
	rabbitclient "github.com/jfgrea27/hazy-rabbit-operator/internal/rabbitclient"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

type RabbitServerNotUp struct{}

func (m *RabbitServerNotUp) Error() string {
	return "Rabbit server not reachable."
}

// HazyZoneReconciler reconciles a HazyZone object
type HazyZoneReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const hazyRabbitFinalizer = "hazy.hazy.com/rabbitcleanup"

//+kubebuilder:rbac:groups=hazy.hazy.com,resources=hazyzonles,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=hazy.hazy.com,resources=hazyzones/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=hazy.hazy.com,resources=hazyzones/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the HazyZone object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *HazyZoneReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)

	hazyzone := &hazyv1alpha1.HazyZone{}
	err := r.Get(ctx, req.NamespacedName, hazyzone)

	// Get the HazyZone
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			log.Info("HazyZone not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get HazyZone")
		return ctrl.Result{}, err
	}

	// get rabbit client and check we can connect
	rclient := rabbitclient.BuildRabbitClient(log)

	if rclient == nil {
		return ctrl.Result{}, &RabbitServerNotUp{}

	}

	ns := hazyzone.GetNamespace()

	exchange := rabbitclient.RabbitExchange{
		ExchangeName: ns,
		Queues:       hazyzone.Spec.Queues,
		VHost:        ns,
		Password:     uuid.New().String(),
	}

	isHazyZoneMarkedToBeDeleted := hazyzone.GetDeletionTimestamp() != nil
	if isHazyZoneMarkedToBeDeleted {
		if controllerutil.ContainsFinalizer(hazyzone, hazyRabbitFinalizer) {
			// Run finalization logic for memcachedFinalizer. If the
			// finalization logic fails, don't remove the finalizer so
			// that we can retry during the next reconciliation.

			if err := r.finalizeRabbitCleanup(log, &exchange, rclient); err != nil {
				return ctrl.Result{}, err
			}

			// Remove memcachedFinalizer. Once all finalizers have been
			// removed, the object will be deleted.
			controllerutil.RemoveFinalizer(hazyzone, hazyRabbitFinalizer)
			err := r.Update(ctx, hazyzone)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// create K8S secret with credentials for users of vHost.
	secretFound := &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{Name: buildSecretName(ns), Namespace: hazyzone.Namespace}, secretFound)
	if err != nil && errors.IsNotFound(err) {
		// Define a new Secret
		sec := r.secretForHazyZone(hazyzone, &exchange, ns)
		log.Info("Creating a new Secret", "Secret.Namespace", sec.Namespace, "Deployment.Name", sec.Name)
		err = r.Create(ctx, sec)
		if err != nil {
			log.Error(err, "Failed to create new Secret", "Secret.Namespace", sec.Namespace, "Secret.Name", sec.Name)
			return ctrl.Result{}, err
		}
	} else if err != nil {
		log.Error(err, "Failed to get Secret")
		return ctrl.Result{}, err
	}
	// Add rabbit resources
	rclient.SetupRabbit(&exchange)

	return ctrl.Result{}, nil

	// TODO
	// refactor more
	// tests
	// revisit models
}

func buildSecretName(ns string) string {
	return fmt.Sprintf("%v-rabbit-sec", ns)
}

// secretForHazyZone returns a hazyZone Secret object
func (r *HazyZoneReconciler) secretForHazyZone(hz *hazyv1alpha1.HazyZone, exchange *rabbitclient.RabbitExchange, ns string) *corev1.Secret {
	sec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      buildSecretName(ns),
			Namespace: ns,
		},
		StringData: map[string]string{
			"RABBIT_USER":     exchange.VHost,
			"RABBIT_PASSWORD": exchange.Password,
		},
	}
	// Set Secrtet instance as the owner and controller
	ctrl.SetControllerReference(hz, sec, r.Scheme)
	return sec
}

func labelsForMemcached(name string) map[string]string {
	return map[string]string{"app": "memcached", "memcached_cr": name}
}

func (r *HazyZoneReconciler) finalizeRabbitCleanup(log logr.Logger, exchange *rabbitclient.RabbitExchange, rclient *rabbitclient.RabbitClient) error {
	log.Info("Successfully finalized rabbit cleanup.")
	rclient.TearDownRabbit(exchange)

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *HazyZoneReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&hazyv1alpha1.HazyZone{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}
