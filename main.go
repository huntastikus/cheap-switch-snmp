package main

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/shirou/gopsutil/host"
	"github.com/soniah/gosnmp"
	"gopkg.in/yaml.v3"
)

type Switch struct {
	Address  string `yaml:"address"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	Name     string `yaml:"name"`
}

type Config struct {
	Switches      []Switch `yaml:"switches"`
	SNMPPort      uint16   `yaml:"snmp_port"`
	SNMPCommunity string   `yaml:"snmp_community"`
}

type Port struct {
	Name       string
	State      string
	LinkStatus string
	TxGoodPkt  int
	TxBadPkt   int
	RxGoodPkt  int
	RxBadPkt   int
}

const (
	baseOID       = ".1.3.6.1.4.1.12345"
	portStateOID  = baseOID + ".1"
	linkStateOID  = baseOID + ".2"
	txGoodPktOID  = baseOID + ".3"
	txBadPktOID   = baseOID + ".4"
	rxGoodPktOID  = baseOID + ".5"
	rxBadPktOID   = baseOID + ".6"
)

var (
	portStats = make(map[string]map[string]Port)
	statsMux  sync.RWMutex
)

func main() {
	config, err := readConfig("config.yaml")
	if err != nil {
		log.Fatal("Error reading configuration:", err)
	}

	for _, sw := range config.Switches {
		portStats[sw.Name] = make(map[string]Port)
	}

	for _, sw := range config.Switches {
		go collectSwitchStats(sw)
	}

	snmp := &gosnmp.GoSNMP{
		Port:      config.SNMPPort,
		Community: config.SNMPCommunity,
		Version:   gosnmp.Version2c,
		Timeout:   time.Duration(2) * time.Second,
	}

	log.Printf("Starting SNMP agent on port %d", config.SNMPPort)
	err = snmp.Listen()
	if err != nil {
		log.Fatal(err)
	}

	snmp.Handler = snmpHandler

	select {}
}

func collectSwitchStats(sw Switch) {
	client := &http.Client{}
	baseURL := "http://" + sw.Address + "/port.cgi?page=stats"
	
	ticker := time.NewTicker(30 * time.Second)
	for range ticker.C {
		stats, err := fetchSwitchStats(client, sw, baseURL)
		if err != nil {
			log.Printf("Error collecting stats from %s (%s): %v", sw.Name, sw.Address, err)
			continue
		}

		statsMux.Lock()
		portStats[sw.Name] = stats
		statsMux.Unlock()
	}
}

func snmpHandler(packet *gosnmp.SnmpPacket) (*gosnmp.SnmpPacket, error) {
	response := &gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{},
	}

	statsMux.RLock()
	defer statsMux.RUnlock()

	for switchName, ports := range portStats {
		for portName, port := range ports {
			oid := fmt.Sprintf("%s.%s.%s", baseOID, switchName, portName)
			
			response.Variables = append(response.Variables,
				gosnmp.SnmpPDU{
					Name:  oid + ".state",
					Type:  gosnmp.Integer,
					Value: stateToInt(port.State),
				},
				gosnmp.SnmpPDU{
					Name:  oid + ".linkStatus",
					Type:  gosnmp.Integer,
					Value: linkStatusToInt(port.LinkStatus),
				},
				gosnmp.SnmpPDU{
					Name:  oid + ".txGoodPkt",
					Type:  gosnmp.Counter64,
					Value: uint64(port.TxGoodPkt),
				},
				gosnmp.SnmpPDU{
					Name:  oid + ".txBadPkt",
					Type:  gosnmp.Counter64,
					Value: uint64(port.TxBadPkt),
				},
				gosnmp.SnmpPDU{
					Name:  oid + ".rxGoodPkt",
					Type:  gosnmp.Counter64,
					Value: uint64(port.RxGoodPkt),
				},
				gosnmp.SnmpPDU{
					Name:  oid + ".rxBadPkt",
					Type:  gosnmp.Counter64,
					Value: uint64(port.RxBadPkt),
				},
			)
		}
	}

	return response, nil
}

func stateToInt(state string) int {
	if state == "Enable" {
		return 1
	}
	return 0
}

func linkStatusToInt(status string) int {
	if status == "Link Up" {
		return 1
	}
	return 0
}

func getMD5Hash(text string) string {
	hash := md5.Sum([]byte(text))
	return hex.EncodeToString(hash[:])
}

func readConfig(filename string) (Config, error) {
	var config Config
	data, err := os.ReadFile(filename)
	if err != nil {
		return config, err
	}
	err = yaml.Unmarshal(data, &config)
	return config, err
}
