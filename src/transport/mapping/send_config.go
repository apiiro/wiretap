package mapping

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"

	"github.com/spf13/viper"
)

type HostConfigurationRequest struct {
	MappedOrder int
	Host        string
}

type NetworkBrokerConfigurationRequest struct {
	Hosts        []HostConfigurationRequest
	MappedPrefix string
}

func SendConfig(hostsMapping []HostMapping, mappingPrefix string) {
	endpoint := viper.GetString("Config.Endpoint")
	accessToken := viper.GetString("Config.Token")

	hosts := []HostConfigurationRequest{}
	for i, host := range hostsMapping {
		hosts = append(hosts, HostConfigurationRequest{
			Host:        host.Host,
			MappedOrder: i + 1,
		})
	}

	configRequest := NetworkBrokerConfigurationRequest{
		Hosts:        hosts,
		MappedPrefix: mappingPrefix,
	}

	err := sendRequest(endpoint, accessToken, configRequest)
	if err != nil {
		log.Println("Error sending config:", err)
	}
}

func sendRequest(endpoint, accessToken string, data NetworkBrokerConfigurationRequest) error {
	// Prepare request
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	log.Println("Config Json", string(jsonData))

	req, err := http.NewRequest(http.MethodPut, endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	// Send request
	client := http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Handle response
	log.Println("Response Status:", resp.Status)
	return nil
}
