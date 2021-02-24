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

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/klog"
	"k8s.io/klog/klogr"
	ctrl "sigs.k8s.io/controller-runtime"

	vpcv1alpha1 "github.com/Sh4d1/scaleway-k8s-vpc/api/v1alpha1"
	"github.com/Sh4d1/scaleway-k8s-vpc/nodes"
	"github.com/Sh4d1/scaleway-k8s-vpc/pkg/nics"
	instance "github.com/scaleway/scaleway-sdk-go/api/instance/v1"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")

	cacheUpdateFrequency = time.Minute * 20
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = vpcv1alpha1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	klog.InitFlags(nil)
	flag.Parse()

	ctrl.SetLogger(klogr.New())

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		Port:               9443,
		LeaderElection:     false,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	metadataAPI := instance.NewMetadataAPI()
	md, err := metadataAPI.GetMetadata()
	if err != nil {
		setupLog.Error(err, "unable to fetch Scaleway metdata")
		os.Exit(1)
	}

	nodeName := os.Getenv("NODE_NAME")
	if nodeName == "" {
		setupLog.Info("Node name not specified, using hostname")
		nodeName = md.Hostname
	}

	macs := []string{}
	for _, pn := range md.PrivateNICs {
		macs = append(macs, pn.MacAddress)
	}

	nics, err := nics.NewNICs(macs)
	if err != nil {
		setupLog.Error(err, "unable to init nics handler")
		os.Exit(1)
	}

	if err = (&nodes.NetworkInterfaceReconciler{
		Client:      mgr.GetClient(),
		Log:         ctrl.Log.WithName("controllers").WithName("NetworkInterface"),
		Scheme:      mgr.GetScheme(),
		MetadataAPI: metadataAPI,
		NodeName:    nodeName,
		NICs:        nics,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "NetworkInterface")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
