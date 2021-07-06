package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"

	"github.com/cloudflare/cloudflare-go"
	"github.com/spf13/viper"
)

type Configuration struct {
	CloudflareToken string `json:"cloudflare_token"`
	Zone            string `json:"zone"`
	Subdomain       string `json:"subdomain"`
}

var (
	configFile *string
	config     Configuration
)

func main() {
	log.Println("Starting CloudFlare DDNS Client!")

	// Read configuration
	configFile = flag.String("config", "", "Config file path")
	flag.Parse()

	if configFile != nil {
		viper.SetConfigFile(*configFile)
	} else {
		viper.SetConfigName("config")
		viper.AddConfigPath(".")
		viper.AddConfigPath("/")
	}

	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("Error reading config file, %s", err)
	}

	if err := viper.Unmarshal(&config); err != nil {
		log.Fatalf("unable to decode into struct, %v", err)
	}

	record_name := fmt.Sprintf("%s.%s", config.Subdomain, config.Zone)

	// Initialize CloudFlare API client
	api, err := cloudflare.NewWithAPIToken(config.CloudflareToken)
	if err != nil {
		log.Fatal(err)
	}

	// Retrieve Public IP
	resp, err := http.Get("https://www.cloudflare.com/cdn-cgi/trace")
	if err != nil {
		log.Fatalf("Unable to get public ip, %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Unable to decode payload, %v", err)
	}

	r, _ := regexp.Compile(`(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)(\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)){3}`)
	publicIP := r.FindString(string(body))
	if publicIP == "" {
		log.Fatalln("Could not retrieve public IP")
	}

	log.Printf("Public IP: %s \n", publicIP)

	// Retrieve Record
	ctx := context.Background()
	zoneID, err := api.ZoneIDByName(config.Zone)
	if err != nil {
		panic(err)
	}
	log.Printf("Zone ID for %s: %s\n", config.Zone, zoneID)

	records, err := api.DNSRecords(ctx, zoneID, cloudflare.DNSRecord{Name: record_name, Type: "A"})
	if err != nil {
		panic(err)
	}

	// Create record if not found
	if len(records) == 0 {
		log.Println("No record found")
		var proxied = true
		_, err = api.CreateDNSRecord(ctx, zoneID, cloudflare.DNSRecord{
			Type:    "A",
			Name:    record_name,
			Content: publicIP,
			Proxied: &proxied,
			TTL:     1,
		})
		if err != nil {
			panic(err)
		}
		os.Exit(0)

	}

	// Check if record content is up to date
	record := records[0]
	if record.Content == publicIP {
		log.Println("SAME IP")
	} else {
		record.Content = publicIP
	}

	// Update record content if required
	err = api.UpdateDNSRecord(ctx, zoneID, record.ID, record)
	if err != nil {
		panic(err)
	}

	log.Printf("Success")
}
