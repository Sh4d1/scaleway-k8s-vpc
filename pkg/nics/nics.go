package nics

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"

	"github.com/vishvananda/netlink"
)

const (
	dhcpcdRunFilePrefix = "/var/run/dhcpcd-"
	dhcpcdRunFileSuffix = "-4.pid"
)

var (
	nicNotFoundErr = errors.New("NIC not found")
)

type Route struct {
	To  *net.IPNet
	Via net.IP
}

func (r Route) isIn(routes []netlink.Route) bool {
	for _, route := range routes {
		if route.Dst.String() == r.To.String() && route.Gw.Equal(r.Via) {
			return true
		}
	}
	return false
}

func isIn(r netlink.Route, routes []Route) bool {
	for _, route := range routes {
		if r.Dst.String() == route.To.String() && r.Gw.Equal(route.Via) {
			return true
		}
	}
	return false
}

type NICs struct {
	Handle *netlink.Handle
	Links  map[string]netlink.Link
}

func NewNICs(macs []string) (*NICs, error) {
	handle, err := netlink.NewHandle()
	if err != nil {
		return nil, err
	}

	nics := &NICs{
		Handle: handle,
		Links:  make(map[string]netlink.Link),
	}

	links, err := handle.LinkList()
	if err != nil {
		return nil, err
	}

	for _, link := range links {
		for _, mac := range macs {
			if link.Attrs().HardwareAddr.String() == mac {
				nics.Links[mac] = link
				break
			}
		}
	}

	return nics, nil
}

func (n *NICs) GetLinkName(mac string) (string, error) {
	link, err := n.getLink(mac)
	if err != nil {
		return "", err
	}
	return link.Attrs().Name, nil
}

func (n *NICs) getLink(mac string) (netlink.Link, error) {
	if link, ok := n.Links[mac]; ok {
		return link, nil
	}

	links, err := n.Handle.LinkList()
	if err != nil {
		return nil, err
	}

	for _, link := range links {
		if link.Attrs().HardwareAddr.String() == mac {
			n.Links[mac] = link
			return link, nil
		}
	}

	return nil, fmt.Errorf("link with address %s: %w", mac, nicNotFoundErr)
}

func maskEqual(m1, m2 net.IPMask) bool {
	if len(m1) != len(m2) {
		return false
	}

	for i, v := range m1 {
		if m2[i] != v {
			return false
		}
	}
	return true
}

func (n *NICs) ConfigureDHCPLink(mac string) (string, error) {
	link, err := n.getLink(mac)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(dhcpcdRunFilePrefix + link.Attrs().Name + dhcpcdRunFileSuffix); err != nil {
		if !os.IsNotExist(err) {
			return "", err
		}
		cmd := exec.Command("dhcpcd", "-A4", "--waitip", "-C", "resolv.conf", "-G", link.Attrs().Name)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return "", err
		}
	}

	err = netlink.LinkSetUp(link)
	if err != nil {
		return "", err
	}

	addrs, err := netlink.AddrList(link, netlink.FAMILY_V4)
	if err != nil {
		return "", err
	}

	if len(addrs) != 1 {
		return "", fmt.Errorf("found %d address for link %s instead of 1", len(addrs), link.Attrs().Name)
	}

	return addrs[0].IP.String(), nil
}

func (n *NICs) ConfigureStaticLink(mac string, ip string) error {
	link, err := n.getLink(mac)
	if err != nil {
		return err
	}

	ipnet, err := netlink.ParseIPNet(ip)
	if err != nil {
		return err
	}

	addrs, err := netlink.AddrList(link, netlink.FAMILY_ALL)
	if err != nil {
		return err
	}

	ipFound := false
	for _, addr := range addrs {
		if maskEqual(addr.IPNet.Mask, ipnet.Mask) && addr.IPNet.IP.Equal(ipnet.IP) {
			ipFound = true
			break
		}
	}

	if !ipFound {
		err := netlink.AddrAdd(link, &netlink.Addr{
			IPNet: ipnet,
		})
		if err != nil {
			return err
		}
	}

	err = netlink.LinkSetUp(link)
	if err != nil {
		return err
	}
	return nil
}

func (n *NICs) TearDownDHCPLink(mac string) error {
	link, err := n.getLink(mac)
	if err != nil {
		if errors.Is(err, nicNotFoundErr) {
			return nil
		}
		return err
	}

	_, err = os.Stat(dhcpcdRunFilePrefix + link.Attrs().Name + dhcpcdRunFileSuffix)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	if err == nil {
		cmd := exec.Command("dhcpcd", "-A4", "--waitip", "-C", "resolv.conf", "-G", "-k", link.Attrs().Name)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
	}

	err = netlink.LinkSetDown(link)
	if err != nil {
		return err
	}
	return nil
}

func (n *NICs) TearDownStaticLink(mac string, ip string) error {
	link, err := n.getLink(mac)
	if err != nil {
		if errors.Is(err, nicNotFoundErr) {
			return nil
		}
		return err
	}

	ipnet, err := netlink.ParseIPNet(ip)
	if err != nil {
		return err
	}

	addrs, err := netlink.AddrList(link, netlink.FAMILY_ALL)
	if err != nil {
		return err
	}

	ipFound := false
	for _, addr := range addrs {
		if maskEqual(addr.IPNet.Mask, ipnet.Mask) && addr.IPNet.IP.Equal(ipnet.IP) {
			ipFound = true
			break
		}
	}

	if ipFound {
		err := netlink.AddrDel(link, &netlink.Addr{
			IPNet: ipnet,
		})
		if err != nil {
			return err
		}
	}

	err = netlink.LinkSetDown(link)
	if err != nil {
		return err
	}
	return nil
}

func (n *NICs) SyncRoutes(mac string, routes []Route) error {
	link, err := n.getLink(mac)
	if err != nil {
		return err
	}

	existingRoutes, err := netlink.RouteList(link, netlink.FAMILY_ALL)
	if err != nil {
		return err
	}

	for _, existingRoute := range existingRoutes {
		if !isIn(existingRoute, routes) && existingRoute.Src == nil {
			err := netlink.RouteDel(&existingRoute)
			if err != nil {
				return err
			}
		}
	}

	for _, route := range routes {
		if !route.isIn(existingRoutes) {
			err := netlink.RouteAdd(&netlink.Route{
				LinkIndex: link.Attrs().Index,
				Dst:       route.To,
				Gw:        route.Via,
			})
			if err != nil {
				return err
			}
		}
	}
	return nil
}
