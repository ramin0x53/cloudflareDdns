package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/huin/goupnp/dcps/internetgateway2"
)

var duration = 30

type CloudFlare struct {
	Token    string
	ZoneId   string
	Dns      string
	RecordId string
}

func NewCloudFlare(token, zoneId, domain string) (*CloudFlare, error) {
	cloudFlare := CloudFlare{token, zoneId, domain, ""}
	var err error
	cloudFlare.RecordId, err = cloudFlare.GetRecordId()
	if err != nil {
		return nil, err
	}
	log.Println("the dns record id is " + cloudFlare.RecordId)
	return &cloudFlare, nil
}

func (c *CloudFlare) GetToken() string {
	return "Bearer " + c.Token
}

func (c *CloudFlare) GetRecordId() (string, error) {
	respBody := struct {
		Result []struct {
			Id   string `json:"id"`
			Name string `json:"name"`
		} `json:"result"`
		Success bool `json:"Success"`
	}{}

	url := "https://api.cloudflare.com/client/v4/zones/" + c.ZoneId + "/dns_records"
	method := "GET"

	client := &http.Client{Timeout: time.Duration(duration) * time.Second}
	req, err := http.NewRequest(method, url, nil)

	if err != nil {
		return "", err
	}
	req.Header.Add("Authorization", c.GetToken())

	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}

	err = json.Unmarshal(body, &respBody)
	if err != nil {
		return "", err
	}

	if !respBody.Success || res.StatusCode >= 300 || res.StatusCode < 200 {
		return "", errors.New("Failed to get dns record id")
	}

	recordId := ""

	for _, res := range respBody.Result {
		if res.Name == c.Dns {
			recordId = res.Id
			continue
		}
	}

	return recordId, nil
}

func (c *CloudFlare) SetIp(ip string) error {
	url := "https://api.cloudflare.com/client/v4/zones/" + c.ZoneId + "/dns_records/" + c.RecordId
	method := "PUT"

	payload := strings.NewReader("{\"name\": \"" + c.Dns + "\",\"ttl\": 60,\"type\": \"A\",\"content\": \"" + ip + "\",\"proxied\": false}")

	client := &http.Client{Timeout: time.Duration(duration-5) * time.Second}
	req, err := http.NewRequest(method, url, payload)
	if err != nil {
		return err
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", c.GetToken())

	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	respBody := struct {
		Success bool `json:"success"`
	}{}

	err = json.Unmarshal(body, &respBody)
	if err != nil {
		return err
	}

	if !respBody.Success || res.StatusCode >= 300 || res.StatusCode < 200 {
		return errors.New("Failed to sed dns ip")
	}

	return nil
}

func getIp() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(duration-5)*time.Second)
	defer cancel()

	ip1Clients, _, err := internetgateway2.NewWANIPConnection1ClientsCtx(ctx)
	if err != nil {
		return "", err
	}

	if len(ip1Clients) <= 0 {
		return "", errors.New("couldn't find wan ip")
	}

	externalIP, err := ip1Clients[0].GetExternalIPAddress()
	if err != nil {
		return "", err
	}

	return externalIP, err
}

func UpdateIp(CloudFlare *CloudFlare) {
	log.Println("start updating dns: " + CloudFlare.Dns)
	defer log.Println("end updating dns: " + CloudFlare.Dns)

	ip, err := getIp()
	if err != nil {
		log.Println("error: ", err)
		return
	}
	log.Println("wan ip is " + ip)

	err = CloudFlare.SetIp(ip)
	if err != nil {
		log.Println("error: ", err)
		return
	}
	log.Println(CloudFlare.Dns + " has been set to " + ip)
}

func main() {
	token, exists := os.LookupEnv("TOKEN")
	if !exists {
		log.Fatal("TOKEN env is undefined")
	}

	zoneId, exists := os.LookupEnv("ZONEID")
	if !exists {
		log.Fatal("ZONEID env is undefined")
	}

	domain, exists := os.LookupEnv("DOMAIN")
	if !exists {
		log.Fatal("DOMAIN env is undefined")
	}

	durationEnv, exists := os.LookupEnv("DURATION")
	if exists {
		var err error
		duration, err = strconv.Atoi(durationEnv)
		if err != nil {
			log.Fatal(err)
		}
	}

	if duration <= 5 {
		log.Panicln("update duration should be more than 5s")
	}

	log.Printf("update duration is %ds\n", duration)

	cloudFlare, err := NewCloudFlare(token, zoneId, domain)
	if err != nil {
		log.Fatal(err)
	}

	s, err := gocron.NewScheduler()
	if err != nil {
		log.Fatal(err)
	}

	UpdateIp(cloudFlare)
	_, err = s.NewJob(
		gocron.DurationJob(
			time.Duration(duration)*time.Second,
		),
		gocron.NewTask(
			UpdateIp,
			cloudFlare,
		),
	)
	if err != nil {
		log.Fatal(err)
	}

	s.Start()

	select {}
}
