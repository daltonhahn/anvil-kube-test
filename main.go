package main

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"fmt"
	"context"
	"sync"
	"bytes"
	"time"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"encoding/json"
	"math/rand"
	"github.com/gorilla/mux"
)

var server *http.Server
var route *mux.Router

var IsLead bool = false
var IsTLS bool = false
var Members []string
var Identity string

var GossipSendCount int = 0
var GossipRecvCount int = 0
var HeartbeatSendCount int = 0
var HeartbeatRecvCount int = 0

var S = rand.NewSource(time.Now().Unix())
var R = rand.New(S)

func getConfig() (*tls.Config) {
	curDir, _ := os.Getwd()

	caCertPool := x509.NewCertPool()
        caCert, err := ioutil.ReadFile(curDir+"/certs/ca.crt")
	if err != nil {
		log.Printf("Read file error #%v", err)
        }
        caCertPool.AppendCertsFromPEM(caCert)

	tlsConfig := &tls.Config{}
        tlsConfig.Certificates = make([]tls.Certificate, 1)

        tlsConfig.Certificates[0], err = tls.LoadX509KeyPair(curDir+"/certs/"+Identity+".crt", curDir+"/certs/"+Identity+".key")
        if err != nil {
                log.Fatal("YOOOOOOOO ", err)
        }
        tlsConfig.BuildNameToCertificate()

	keyPairs := tlsConfig.Certificates
	caCerts := caCertPool

	myConf := &tls.Config{
                ClientCAs:              caCerts,
                Certificates:           keyPairs,
        }
	return myConf
}

func startTLS() {
	//Add some logic to make sure that you're assigning the correct cert to this node 
        route := mux.NewRouter()
        registerRoutes(route)
	newserver := &http.Server {
		Addr: ":8080",
		TLSConfig: getConfig(),
		Handler: route,
	}

	//log.Println(server.ListenAndServeTLS(curDir + "/certs/"+Identity+".crt", curDir+"/certs/"+Identity+".key"))
	if err := newserver.ListenAndServeTLS("", ""); err != nil {
		log.Println("YOOOOOOOO ", err)
	}
}

func main() {
	wg := new(sync.WaitGroup)
	wg.Add(2)
        route := mux.NewRouter()
        registerRoutes(route)
	server = &http.Server{
                Addr: ":8080",
                Handler: route,
        }
	go func() {
		log.Println(server.ListenAndServe())
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
	route.HandleFunc("/", HelloWorld).Methods("GET")
}

func HelloWorld(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Hello World\n")
}

func Starter(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	starterBlock := struct {
		Lead bool
		TLS bool
		Mems []string
		Identity string
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
	Identity = starterBlock.Identity
	fmt.Fprintf(w, "Config: %s\n", starterBlock)
	go setupDaemon()
}

func setupDaemon() {
	if IsTLS == true {
		ctxShutDown, cancel := context.WithTimeout(context.Background(), (5*time.Second))
		defer func() {
			cancel()
		}()
		if err := server.Shutdown(ctxShutDown); err != nil {
			log.Printf("server Shutdown Failed:%+s", err)
		}
		if err := server.Shutdown(ctxShutDown); err != nil {
			log.Printf("server Shutdown Failed:%+s", err)
                }
		go startTLS()
	}
	time.Sleep(10*time.Second)
	registerUDP()
	if IsLead == true {
		go heartbeatSend()
	}
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

		message := string(p[:n])
		if len(message) == 0 {
			log.Fatalln("Message empty ", err)
		}

		if strings.Contains(message, "GOSSIP -- ") {
			fmt.Println("R: Pong")
			GossipRecvCount = GossipRecvCount + 1
			encMessage := "GACK --- " + strconv.Itoa(GossipRecvCount)
			ser.WriteTo([]byte(encMessage), remoteaddr)
		}
	}
}


/*
        rotFlag = true
        go func() {
                for {
                        <-sigHandle
                        if rotFlag == true {
                                rotFlag = false
                                go cw.startNewServer(anv_router)
                        }
                }
                wg.Done()
        }()
        go cw.startNewServer(anv_router)
	*/
