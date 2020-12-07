package nics

import (
	"fmt"
	"net"

	"github.com/vishvananda/netlink"
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

	return nil, fmt.Errorf("nic with MAC address %s not found", mac)
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

func (n *NICs) ConfigureLink(mac string, ip string) error {
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

func (n *NICs) TearDownLink(mac string, ip string) error {
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
