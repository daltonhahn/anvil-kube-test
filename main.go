package main

import (
	"fmt"
	"sync"
	"bytes"
	"time"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"strconv"
	"encoding/json"
	"math/rand"
	"github.com/gorilla/mux"
)

var IsLead bool = false
var IsTLS bool = false
var Members []string

var GossipSendCount int = 0
var GossipRecvCount int = 0
var HeartbeatSendCount int = 0
var HeartbeatRecvCount int = 0

var S = rand.NewSource(time.Now().Unix())
var R = rand.New(S)

func main() {
	wg := new(sync.WaitGroup)
	wg.Add(1)
        route := mux.NewRouter()
        registerRoutes(route)

	go func() {
		log.Println(http.ListenAndServe(":8080", route))
		wg.Done()
	}()
	wg.Wait()
}

func registerUDP() {
        p := make([]byte, 4096)
        addr := net.UDPAddr{
                Port: 8080,
                IP: net.ParseIP("0.0.0.0"),
        }
        ser, err := net.ListenUDP("udp", &addr)
        if err != nil {
                log.Fatalln("Some error %v\n", err)
        }
        go recvGossip(p, ser)
        go sendGossip(ser)
}

func registerRoutes(route *mux.Router) {
        route.HandleFunc("/starter", Starter).Methods("POST")
        route.HandleFunc("/heartbeat", heartbeatRecv).Methods("POST")
}

func Starter(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	starterBlock := struct {
		Lead bool
		TLS bool
		Mems []string
	}{}
	err := decoder.Decode(&starterBlock)
        if err != nil {
		log.Fatalln(err)
        }
	if starterBlock.Lead == true {
		IsLead = true
	} else {
		IsLead = false
	}

	if starterBlock.TLS == true {
		IsTLS = true
	} else {
		IsTLS = false
	}
	Members = starterBlock.Mems
	time.Sleep(10*time.Second)

	registerUDP()
	if IsLead == true {
		go heartbeatSend()
	}

	fmt.Fprintf(w, "Config: %s\n", starterBlock)
}

func heartbeatRecv(w http.ResponseWriter, r *http.Request) {
        b, err := ioutil.ReadAll(r.Body)
        defer r.Body.Close()
        if err != nil {
		log.Fatalln(err)
        }
	heartbeatMessage := struct {
		Time string
		Message string
	}{}
        err = json.Unmarshal(b, &heartbeatMessage)
        if err != nil {
		log.Fatalln(err)
        }
	HeartbeatRecvCount = HeartbeatRecvCount + 1
	fmt.Println("R: Beat")

        var jsonData []byte
        jsonData, err = json.Marshal("HACK --- " + strconv.Itoa(HeartbeatRecvCount))
        if err != nil {
                log.Fatalln("Unable to marshal JSON")
        }
        w.Header().Set("Content-Type", "application/json")
        fmt.Fprintf(w, string(jsonData))
}

func heartbeatSend() {
	time.Sleep(1*time.Second)
	for {
		for _, ele := range Members {
			myTime := time.Now()
			HeartbeatSendCount = HeartbeatSendCount + 1
			msg := strconv.Itoa(HeartbeatSendCount)
			fmt.Println("S: Beat")

			HBMsg := struct {
				Time time.Time
				Msg string
			}{myTime, msg}

			var jsonData []byte
			jsonData, err := json.Marshal(HBMsg)
			if err != nil {
				log.Fatalln("Unable to marshal JSON for HB Send")
			}
			postBody := bytes.NewBuffer(jsonData)
			resp, err := http.Post("http://"+ele+":8080/heartbeat", "application/json", postBody)
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil || len(body) == 0 {
				log.Fatalln(err)
			}
		}
		time.Sleep(1*time.Second)
	}
}

func sendGossip(conn *net.UDPConn) {
        time.Sleep(200 * time.Millisecond)
        for {
		target := Members[R.Intn(len(Members))]
		addr, err := net.ResolveUDPAddr("udp", target+":8080")
		if err != nil {
			log.Fatalln("Invalid IP address")
		}
		fmt.Println("S: Ping")
		myTime := time.Now()
		encMessage := ("GOSSIP -- " + myTime.String())
		conn.WriteTo([]byte(encMessage), addr)
		GossipSendCount = GossipSendCount + 1
                time.Sleep(200 * time.Millisecond)
        }
}

func recvGossip(p []byte, ser *net.UDPConn) {
	for {
		n,remoteaddr,err:= ser.ReadFromUDP(p)

		if err != nil {
			log.Fatalln(err)
		}

		fmt.Println("R: Pong")
		message := string(p[:n])
		if len(message) == 0 {
			log.Fatalln("Message empty ", err)
		}
		GossipRecvCount = GossipRecvCount + 1
		encMessage := "GACK --- " + strconv.Itoa(GossipRecvCount)
		ser.WriteTo([]byte(encMessage), remoteaddr)
	}
}
