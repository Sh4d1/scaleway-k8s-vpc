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

package main

import (
	"flag"
	"os"
	"time"

	goipam "github.com/metal-stack/go-ipam"
	instance "github.com/scaleway/scaleway-sdk-go/api/instance/v1"
	vpc "github.com/scaleway/scaleway-sdk-go/api/vpc/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/klog"
	"k8s.io/klog/klogr"
	ctrl "sigs.k8s.io/controller-runtime"

	vpcv1alpha1 "github.com/Sh4d1/scaleway-k8s-vpc/api/v1alpha1"
	"github.com/Sh4d1/scaleway-k8s-vpc/controllers"
	"github.com/Sh4d1/scaleway-k8s-vpc/pkg/ipam"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")

	defaultCmName        = "scaleway-k8s-vpc-ipam"
	defaultCmNamespace   = "default"
	cacheUpdateFrequency = time.Minute * 20
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = vpcv1alpha1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	klog.InitFlags(nil)
	flag.Parse()

	ctrl.SetLogger(klogr.New())

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		Port:               9443,
		LeaderElection:     enableLeaderElection,
		LeaderElectionID:   "be46b6df.scaleway.com",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	scwClient, err := scw.NewClient(
		scw.WithEnv(),
		scw.WithUserAgent("scaleway-k8s-vpc"),
	)
	if err != nil {
		setupLog.Error(err, "unable to init scaleway client")
	}

	stopCh := ctrl.SetupSignalHandler()

	cmNamespace := os.Getenv("CONFIGMAP_NAMESPACE")
	if cmNamespace == "" {
		cmNamespace = defaultCmNamespace
	}
	cmName := os.Getenv("CONFIGMAP_NAME")
	if cmName == "" {
		cmName = defaultCmName
	}

	cmIPAM, err := ipam.NewConfigMapIPAM(types.NamespacedName{
		Name:      cmName,
		Namespace: cmNamespace,
	}, stopCh)
	if err != nil {
		setupLog.Error(err, "error creating ipam storage")
		os.Exit(1)
	}
	ipam := goipam.NewWithStorage(cmIPAM)

	if err = (&controllers.PrivateNetworkReconciler{
		Client:      mgr.GetClient(),
		Log:         ctrl.Log.WithName("controllers").WithName("PrivateNetwork"),
		Scheme:      mgr.GetScheme(),
		IPAM:        ipam,
		InstanceAPI: instance.NewAPI(scwClient),
		VpcAPI:      vpc.NewAPI(scwClient),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PrivateNetwork")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	setupLog.Info("starting manager")
	if err := mgr.Start(stopCh); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
