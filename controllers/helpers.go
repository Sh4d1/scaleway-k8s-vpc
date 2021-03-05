package controllers

import (
	"fmt"

	instance "github.com/scaleway/scaleway-sdk-go/api/instance/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
	corev1 "k8s.io/api/core/v1"
)

func getServerFromNode(instanceAPI *instance.API, node *corev1.Node) (*instance.Server, error) {
	instanceID := ""
	zone := ""
	if node.Spec.ProviderID != "" {
		providerID := node.Spec.ProviderID
		if providerIDRegexp.MatchString(providerID) {
			match := providerIDRegexp.FindStringSubmatch(providerID)

			for i, name := range providerIDRegexp.SubexpNames() {
				if i != 0 && name != "" {
					if match[i] != "" {
						switch name {
						case regexpUUID:
							instanceID = match[i]
						case regexpLocalization:
							zone = match[i]
						}
					}
				}
			}
		}
	}

	if instanceID != "" {
		serverResp, err := instanceAPI.GetServer(&instance.GetServerRequest{
			Zone:     scw.Zone(zone),
			ServerID: instanceID,
		})
		if err == nil {
			return serverResp.Server, nil
		}
	}

	serversListResp, err := instanceAPI.ListServers(&instance.ListServersRequest{
		Zone: scw.Zone(zone),
		Name: scw.StringPtr(node.Name),
	})
	if err != nil {
		return nil, err
	}
	if len(serversListResp.Servers) != 1 {
		return nil, fmt.Errorf("found %d servers with name %s instead of 1", len(serversListResp.Servers), node.Name)
	}
	return serversListResp.Servers[0], nil
}
