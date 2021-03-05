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
	"regexp"
	"strings"
	"time"

	"github.com/go-logr/logr"
	goipam "github.com/metal-stack/go-ipam"
	instance "github.com/scaleway/scaleway-sdk-go/api/instance/v1"
	vpc "github.com/scaleway/scaleway-sdk-go/api/vpc/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

const (
	regexpProduct      = "product"
	regexpLocalization = "localization"
	regexpUUID         = "uuid"
)

var (
	providerIDRegexp = regexp.MustCompile(fmt.Sprintf("scaleway://((?P<%s>.*?)/(?P<%s>.*?)/(?P<%s>.*)|(?P<%s>.*))", regexpProduct, regexpLocalization, regexpUUID, regexpUUID))

	// RequeueDuration is the default requeue duration
	RequeueDuration time.Duration = time.Second * 30
)

// PrivateNetworkReconciler reconciles a PrivateNetwork object
type PrivateNetworkReconciler struct {
	client.Client
	Log         logr.Logger
	Scheme      *runtime.Scheme
	IPAM        goipam.Ipamer
	InstanceAPI *instance.API
	VpcAPI      *vpc.API
}

// +kubebuilder:rbac:groups=vpc.scaleway.com,resources=privatenetworks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vpc.scaleway.com,resources=privatenetworks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vpc.scaleway.com,resources=networkinterfaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vpc.scaleway.com,resources=networkinterfaces/status,verbs=get;update
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch

func (r *PrivateNetworkReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("privatenetwork", req.NamespacedName)

	pn := &vpcv1alpha1.PrivateNetwork{}

	err := r.Get(ctx, req.NamespacedName, pn)
	if err != nil {
		log.Error(err, "could not find object")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	prefix, err := r.IPAM.NewPrefix(pn.Spec.CIDR)
	if err != nil {
		log.Error(err, "error creating new prefix")
		return ctrl.Result{}, err
	}

	if !pn.ObjectMeta.GetDeletionTimestamp().IsZero() {
		// deletion
		if controllerutil.ContainsFinalizer(pn, constants.FinalizerName) {
			nicsList := &vpcv1alpha1.NetworkInterfaceList{}
			err = r.Client.List(ctx, nicsList,
				client.MatchingLabels{
					constants.PrivateNetworkLabel: pn.Name,
				},
			)
			if err != nil {
				log.Error(err, fmt.Sprintf("could not list NetworkInterface for privateNetwork %s", pn.Name))
				return ctrl.Result{}, err
			}

			for _, nic := range nicsList.Items {
				if nic.ObjectMeta.GetDeletionTimestamp().IsZero() {
					err := r.Client.Delete(ctx, &nic)
					if err != nil {
						log.Error(err, fmt.Sprintf("failed to delete networkInterface %s", nic.Name))
						return ctrl.Result{}, err
					}
				}
			}
			if len(nicsList.Items) == 0 {
				_, err = r.IPAM.DeletePrefix(pn.Spec.CIDR)
				if err != nil {
					if !errors.As(err, &goipam.NotFoundError{}) {
						log.Error(err, "failed to delete PrivateNetwork prefix")
						return ctrl.Result{}, err
					}
				}
				controllerutil.RemoveFinalizer(pn, constants.FinalizerName)
				if err := r.Update(ctx, pn); err != nil {
					log.Error(err, "failed to add finalizer")
					return ctrl.Result{}, err
				}
				return ctrl.Result{}, nil
			}
			return ctrl.Result{RequeueAfter: RequeueDuration}, nil
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(pn, constants.FinalizerName) {
		controllerutil.AddFinalizer(pn, constants.FinalizerName)
		if err := r.Update(ctx, pn); err != nil {
			log.Error(err, "failed to add finalizer")
			return ctrl.Result{}, err
		}
	}

	_, err = r.VpcAPI.GetPrivateNetwork(&vpc.GetPrivateNetworkRequest{
		Zone:             scw.Zone(pn.Spec.Zone),
		PrivateNetworkID: pn.Spec.ID,
	})
	if err != nil {
		log.Error(err, "error getting private network from api")
		return ctrl.Result{RequeueAfter: RequeueDuration}, err
	}

	nodesList := &corev1.NodeList{}
	err = r.Client.List(ctx, nodesList)
	if err != nil {
		log.Error(err, "could not list nodes")
		return ctrl.Result{RequeueAfter: RequeueDuration}, err
	}

	for _, node := range nodesList.Items {
		nicsList := &vpcv1alpha1.NetworkInterfaceList{}
		err = r.Client.List(ctx, nicsList,
			client.MatchingLabels{
				constants.PrivateNetworkLabel: pn.Name,
				constants.NodeLabel:           node.Name,
			},
		)
		if err != nil {
			log.Error(err, fmt.Sprintf("could not list NetworkInterface for node %s and privateNetwork %s", node.Name, pn.Name))
			return ctrl.Result{RequeueAfter: RequeueDuration}, err
		}

		server, err := getServerFromNode(r.InstanceAPI, &node)
		if err != nil {
			log.Error(err, fmt.Sprintf("could not get scaleway server from node %s", node.Name))
			break
		}

		var privateNIC *instance.PrivateNIC
		for _, pnic := range server.PrivateNics {
			if pnic.PrivateNetworkID == pn.Spec.ID {
				privateNIC = pnic
				break
			}
		}
		if privateNIC == nil {
			pnicResp, err := r.InstanceAPI.CreatePrivateNIC(&instance.CreatePrivateNICRequest{
				Zone:             server.Zone,
				PrivateNetworkID: pn.Spec.ID,
				ServerID:         server.ID,
			})
			if err != nil {
				log.Error(err, fmt.Sprintf("unable to create private on server %s", server.ID))
				return ctrl.Result{RequeueAfter: RequeueDuration}, err
			}
			privateNIC = pnicResp.PrivateNic
		}

		if len(nicsList.Items) > 1 {
			log.Error(fmt.Errorf("node %s have %d networkInterfaces instead of at most one", node.Name, len(nicsList.Items)), "could not handle node")
			return ctrl.Result{RequeueAfter: RequeueDuration}, err
		}

		if len(nicsList.Items) == 0 {
			nic, err := r.constructNetworkInterfaceForPrivateNetwork(pn, node.Name)
			if err != nil {
				log.Error(err, "unable to construct networkInterface from privateNetwork")
				return ctrl.Result{RequeueAfter: RequeueDuration}, err
			}
			ip, err := r.IPAM.AcquireIP(prefix.Cidr)
			if err != nil {
				log.Error(err, fmt.Sprintf("error acquiring ip for cidr %s", prefix.Cidr))
				return ctrl.Result{RequeueAfter: RequeueDuration}, err
			}

			// TODO have a better idea :D
			nic.Spec.Address = ip.IP.String() + "/" + strings.Split(prefix.Cidr, "/")[1]
			nic.Spec.ID = privateNIC.ID
			err = r.Client.Create(ctx, nic)
			if err != nil {
				log.Error(err, "could not create networkInterface")
				return ctrl.Result{RequeueAfter: RequeueDuration}, err
			}
			nic.Status.MacAddress = privateNIC.MacAddress
			err = r.Client.Status().Update(ctx, nic)
			if err != nil {
				log.Error(err, "could not update networkInterface status")
				return ctrl.Result{RequeueAfter: RequeueDuration}, err
			}
			log.Info(fmt.Sprintf("Successfully created networkInterface %s on node %s", nic.Name, node.Name))
		}
	}

	return ctrl.Result{}, nil
}

func (r *PrivateNetworkReconciler) constructNetworkInterfaceForPrivateNetwork(pn *vpcv1alpha1.PrivateNetwork, nodeName string) (*vpcv1alpha1.NetworkInterface, error) {
	nic := &vpcv1alpha1.NetworkInterface{
		ObjectMeta: metav1.ObjectMeta{
			Labels:       make(map[string]string),
			Annotations:  make(map[string]string),
			GenerateName: pn.Name + "-",
		},
		Spec: vpcv1alpha1.NetworkInterfaceSpec{
			NodeName: nodeName,
		},
	}
	for k, v := range pn.Annotations {
		nic.Annotations[k] = v
	}
	for k, v := range pn.Labels {
		nic.Labels[k] = v
	}
	nic.Labels[constants.PrivateNetworkLabel] = pn.Name
	nic.Labels[constants.NodeLabel] = nodeName
	if err := ctrl.SetControllerReference(pn, nic, r.Scheme); err != nil {
		return nil, err
	}
	controllerutil.AddFinalizer(nic, constants.FinalizerName)
	controllerutil.AddFinalizer(nic, constants.IPFinalizerName)

	return nic, nil
}

func (r *PrivateNetworkReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vpcv1alpha1.PrivateNetwork{}).
		Owns(&vpcv1alpha1.NetworkInterface{}).
		Watches(&source.Kind{
			Type: &corev1.Node{},
		}, &handler.Funcs{
			CreateFunc: func(e event.CreateEvent, q workqueue.RateLimitingInterface) {
				pnsList := &vpcv1alpha1.PrivateNetworkList{}
				err := r.Client.List(context.Background(), pnsList)
				if err != nil {
					r.Log.Error(err, "unable to sync privatenetwork on node creation")
					return
				}
				for _, pn := range pnsList.Items {
					q.Add(reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name: pn.Name,
						},
					})
				}
			},
		}).
		Complete(r)
}
