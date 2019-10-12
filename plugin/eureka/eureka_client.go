package eureka

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
)

const (
	statusUp   status = "UP"
	statusDown status = "DOWN"
)

type status string

type serverResponse struct {
	Applications *applications `json:"applications"`
}
type applications struct {
	Application []*application `json:"application"`
}
type application struct {
	Name     string      `json:"name"`
	Instance []*instance `json:"instance"`
}
type instance struct {
	IpAddr     string `json:"ipAddr"`
	VipAddress string `json:"vipAddress"`
	Status     status `json:"status"`
}

type clientAPI interface {
	fetchAllApplications() (*applications, error)
}

type client struct {
	clientAPI

	BaseUrl string
}

func (e client) fetchAllApplications() (*applications, error) {
	req, _ := http.NewRequest(http.MethodGet, e.BaseUrl+"/v2/apps/", nil)
	req.Header.Set("Accept", "application/json")
	client := &http.Client{}
	resp, _ := client.Do(req)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to fetch applications from Eureka, status code: %v", resp.StatusCode)
	}

	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Fatal(readErr)
		return nil, readErr
	}

	eurekaResponse := &serverResponse{}
	jsonErr := json.Unmarshal(body, eurekaResponse)
	if jsonErr != nil {
		log.Fatal(jsonErr)
		return nil, jsonErr
	}
	return eurekaResponse.Applications, nil
}
