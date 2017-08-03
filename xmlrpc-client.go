package main

import (
	"bytes"
	"fmt"
	"net/http"

	"github.com/ochinchina/gorilla-xmlrpc/xml"
)

type XmlRPCClient struct {
	host string
	port int
}

type VersionReply struct {
	Value string
}

type StartStopReply struct {
	Value bool
}

type AllProcessInfoReply struct {
	Value []ProcessInfo
}

func NewXmlRPCClient(host string, port int) *XmlRPCClient {
	return &XmlRPCClient{host: host, port: port}
}

func (r *XmlRPCClient) Url() string {
	return fmt.Sprintf("http://%s:%d/RPC2", r.host, r.port)
}

func (r *XmlRPCClient) GetVersion() (reply VersionReply, err error) {
	ins := struct{}{}
	buf, _ := xml.EncodeClientRequest("supervisor.getVersion", &ins)

	resp, err := http.Post(r.Url(), "text/xml", bytes.NewBuffer(buf))
	if err != nil {
		return
	}
	defer resp.Body.Close()

	err = xml.DecodeClientResponse(resp.Body, &reply)

	return
}

func (r *XmlRPCClient) GetAllProcessInfo() (reply AllProcessInfoReply, err error) {
	ins := struct{}{}
	buf, _ := xml.EncodeClientRequest("supervisor.getAllProcessInfo", &ins)

	resp, err := http.Post(r.Url(), "text/xml", bytes.NewBuffer(buf))
	if err != nil {
		return
	}
	defer resp.Body.Close()

	err = xml.DecodeClientResponse(resp.Body, &reply)

	return
}

func (r *XmlRPCClient) ChangeProcessState(change string, processName string) (reply StartStopReply, err error) {
	if !(change == "start" || change == "stop") {
		err = fmt.Errorf("Incorrect required state")
		return
	}

	ins := struct{ Value string }{processName}
	buf, _ := xml.EncodeClientRequest(fmt.Sprintf("supervisor.%sProcess", change), &ins)

	resp, err := http.Post(r.Url(), "text/xml", bytes.NewBuffer(buf))
	if err != nil {
		return
	}
	defer resp.Body.Close()

	err = xml.DecodeClientResponse(resp.Body, &reply)

	return
}
