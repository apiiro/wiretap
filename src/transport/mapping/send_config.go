package mapping

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
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
	domain := viper.GetString("Apiiro.Domain")
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

	err := sendRequest(fmt.Sprintf("https://%s/rest-api/v1.0/broker/configuration", domain), accessToken, configRequest)
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

	// Create transport from DefaultTransport
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: viper.GetBool("Skip.Ssl.Verify")} // SSL verification is skipped

	// Create http client with modified transport
	client := &http.Client{Transport: transport}

	// Send request
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Handle response
	log.Println("Response Status:", resp.Status)
	return nil
}
