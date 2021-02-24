/*


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

package controllers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	goipam "github.com/metal-stack/go-ipam"
	instance "github.com/scaleway/scaleway-sdk-go/api/instance/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	vpcv1alpha1 "github.com/Sh4d1/scaleway-k8s-vpc/api/v1alpha1"
	"github.com/Sh4d1/scaleway-k8s-vpc/internal/constants"
)

// NetworkInterfaceReconciler reconciles a NetworkInterface object
type NetworkInterfaceReconciler struct {
	client.Client
	Log         logr.Logger
	Scheme      *runtime.Scheme
	IPAM        goipam.Ipamer
	InstanceAPI *instance.API
}

// +kubebuilder:rbac:groups=vpc.scaleway.com,resources=networkinterfaces,verbs=get;list;watch;update
// +kubebuilder:rbac:groups=vpc.scaleway.com,resources=networkinterfaces/status,verbs=get;update
// +kubebuilder:rbac:groups=vpc.scaleway.com,resources=privatenetworks,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch

func (r *NetworkInterfaceReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("networkinterface", req.NamespacedName)

	nic := &vpcv1alpha1.NetworkInterface{}

	err := r.Client.Get(ctx, req.NamespacedName, nic)
	if err != nil {
		log.Error(err, "could not find object")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	node := corev1.Node{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: nic.Spec.NodeName}, &node)
	if err != nil && !apierrors.IsNotFound(err) {
		log.Error(err, "error getting node")
		return ctrl.Result{}, err
	}

	nodeDeleted := err != nil && apierrors.IsNotFound(err)

	if nic.ObjectMeta.GetDeletionTimestamp().IsZero() {
		if nodeDeleted {
			err := r.Client.Delete(ctx, nic)
			if err != nil {
				log.Error(err, fmt.Sprintf("failed to delete networkInterface %s", nic.Name))
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
		}
		// nothing to do
		return ctrl.Result{}, nil
	}

	// nic is deleting

	if controllerutil.ContainsFinalizer(nic, constants.FinalizerName) && nodeDeleted {
		controllerutil.RemoveFinalizer(nic, constants.FinalizerName)
		err = r.Client.Update(ctx, nic)
		if err != nil {
			log.Error(err, fmt.Sprintf("failed to patch networkInterface %s", nic.Name))
			return ctrl.Result{}, err
		}
		//return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
	}

	if !controllerutil.ContainsFinalizer(nic, constants.FinalizerName) {
		pn := vpcv1alpha1.PrivateNetwork{}
		err = r.Client.Get(ctx, types.NamespacedName{Name: nic.OwnerReferences[0].Name}, &pn)
		if err != nil {
			log.Error(err, "unable to get private network")
			return ctrl.Result{}, err
		}

		err := r.IPAM.ReleaseIPFromPrefix(pn.Spec.CIDR, strings.Split(nic.Spec.Address, "/")[0])
		if err != nil {
			if !errors.As(err, &goipam.NotFoundError{}) {
				log.Error(err, fmt.Sprintf("could not delete IP %s from prefix %s", nic.Spec.Address, pn.Spec.CIDR))
				return ctrl.Result{}, err
			}
		}
		node := corev1.Node{}
		err = r.Client.Get(ctx, types.NamespacedName{Name: nic.Spec.NodeName}, &node)
		if err != nil && !apierrors.IsNotFound(err) {
			log.Error(err, "error getting node")
			return ctrl.Result{}, err
		}
		if err == nil {
			server, err := getServerFromNode(r.InstanceAPI, &node)
			if err != nil {
				log.Error(err, "error getting server from node")
				return ctrl.Result{}, err
			}
			privateNicID := ""
			for _, pnic := range server.PrivateNics {
				if pnic.PrivateNetworkID == pn.Spec.ID {
					privateNicID = pnic.ID
					break
				}
			}
			if privateNicID != "" {
				err := r.InstanceAPI.DeletePrivateNIC(&instance.DeletePrivateNICRequest{
					Zone:         server.Zone,
					PrivateNicID: privateNicID,
					ServerID:     server.ID,
				})
				if err != nil {
					log.Error(err, "unable to delete private nic from server")
					return ctrl.Result{}, err
				}
			}
		}

		controllerutil.RemoveFinalizer(nic, constants.IPFinalizerName)
		err = r.Client.Update(ctx, nic)
		if err != nil {
			log.Error(err, fmt.Sprintf("failed to remove finalizer on networkInterface %s", nic.Name))
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *NetworkInterfaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vpcv1alpha1.NetworkInterface{}).
		Watches(&source.Kind{
			Type: &corev1.Node{},
		}, &handler.Funcs{
			DeleteFunc: func(e event.DeleteEvent, q workqueue.RateLimitingInterface) {
				nicsList := &vpcv1alpha1.NetworkInterfaceList{}
				err := r.Client.List(context.Background(), nicsList,
					client.MatchingLabels{
						constants.NodeLabel: e.Meta.GetName(),
					},
				)
				if err != nil {
					r.Log.Error(err, "unable to sync privatenetwork on node creation")
					return
				}
				for _, nic := range nicsList.Items {
					q.Add(reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name: nic.Name,
						},
					})
				}
			},
		}).
		Complete(r)
}
