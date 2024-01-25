package config

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/spf13/viper"
)

type NetworkBrokerKeysResponse struct {
	ApiiroGatewayPublicKey string
}

func GetServerPublicKey() (string, error) {
	domain := viper.GetString("Apiiro.Domain")
	accessToken := viper.GetString("Config.Token")

	response, err := sendRequest(fmt.Sprintf("https://%s/rest-api/v1.0/broker/keys", domain), accessToken)
	if err != nil {
		log.Println("Error getting server public key:", err)
		return "", err
	}

	return response.ApiiroGatewayPublicKey, nil
}

func sendRequest(endpoint, accessToken string) (NetworkBrokerKeysResponse, error) {

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return NetworkBrokerKeysResponse{}, err
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
		return NetworkBrokerKeysResponse{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return NetworkBrokerKeysResponse{}, err
	}

	if resp.StatusCode != http.StatusOK {
		return NetworkBrokerKeysResponse{}, fmt.Errorf("%d %s", resp.StatusCode, string(body))
	}

	// Handle response
	var response NetworkBrokerKeysResponse
	err = json.Unmarshal(body, &response)
	return response, err
}
