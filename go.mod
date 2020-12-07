module github.com/Sh4d1/scaleway-k8s-vpc

go 1.13

require (
	github.com/go-logr/logr v0.1.0
	github.com/metal-stack/go-ipam v1.7.0
	github.com/onsi/ginkgo v1.12.1
	github.com/onsi/gomega v1.10.1
	github.com/scaleway/scaleway-sdk-go v1.0.0-beta.7
	github.com/vishvananda/netlink v1.1.0
	google.golang.org/appengine v1.6.6 // indirect
	k8s.io/api v0.18.6
	k8s.io/apimachinery v0.18.6
	k8s.io/client-go v0.18.6
	k8s.io/klog v1.0.0
	sigs.k8s.io/controller-runtime v0.6.4
)

replace github.com/metal-stack/go-ipam => github.com/Sh4d1/go-ipam v1.7.1-0.20201205075440-d75efe611b90

replace github.com/scaleway/scaleway-sdk-go => github.com/Sh4d1/scaleway-sdk-go v0.0.0-20201205201010-f77f75686f47
