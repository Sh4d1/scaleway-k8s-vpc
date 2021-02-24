module github.com/Sh4d1/scaleway-k8s-vpc

go 1.16

require (
	github.com/coreos/go-iptables v0.5.0
	github.com/go-logr/logr v0.1.0
	github.com/metal-stack/go-ipam v1.8.1
	github.com/onsi/ginkgo v1.12.1
	github.com/onsi/gomega v1.10.1
	github.com/scaleway/scaleway-sdk-go v1.0.0-beta.7.0.20210223165440-c65ae3540d44
	github.com/vishvananda/netlink v1.1.0
	google.golang.org/appengine v1.6.6 // indirect
	k8s.io/api v0.18.6
	k8s.io/apimachinery v0.18.6
	k8s.io/client-go v0.18.6
	k8s.io/klog v1.0.0
	sigs.k8s.io/controller-runtime v0.6.4
)

replace github.com/coreos/go-iptables => github.com/Sh4d1/go-iptables v0.5.1-0.20210224084650-91aadf86de0a
