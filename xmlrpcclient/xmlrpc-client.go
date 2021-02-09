package xmlrpcclient

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/ochinchina/supervisord/types"

	"github.com/ochinchina/gorilla-xmlrpc/xml"
)

// XMLRPCClient the supervisor XML RPC client library
type XMLRPCClient struct {
	serverurl string
	user      string
	password  string
	timeout   time.Duration
	verbose   bool
}

// VersionReply the version reply message from supervisor
type VersionReply struct {
	Value string
}

// StartStopReply the program start/stop reply message from supervisor
type StartStopReply struct {
	Value bool
}

// ShutdownReply the program shutdown reply message
type ShutdownReply StartStopReply

// AllProcessInfoReply all the processes information from supervisor
type AllProcessInfoReply struct {
	Value []types.ProcessInfo
}

var emptyReader io.ReadCloser

func init() {
	var buf bytes.Buffer
	emptyReader = ioutil.NopCloser(&buf)
}

// NewXMLRPCClient create a XMLRPCClient object
func NewXMLRPCClient(serverurl string, verbose bool) *XMLRPCClient {
	return &XMLRPCClient{serverurl: serverurl, timeout: 0, verbose: verbose}
}

// SetUser set the user for basic http auth
func (r *XMLRPCClient) SetUser(user string) {
	r.user = user
}

// SetPassword set the password for basic http auth
func (r *XMLRPCClient) SetPassword(password string) {
	r.password = password
}

// SetTimeout set the http request timeout
func (r *XMLRPCClient) SetTimeout(timeout time.Duration) {
	r.timeout = timeout
}

// URL return the RPC url
func (r *XMLRPCClient) URL() string {
	return fmt.Sprintf("%s/RPC2", r.serverurl)
}

func (r *XMLRPCClient) createHTTPRequest(method string, url string, data interface{}) (*http.Request, error) {
	buf, _ := xml.EncodeClientRequest(method, data)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(buf))
	if err != nil {
		if r.verbose {
			fmt.Println("Fail to create request:", err)
		}
		return nil, err
	}

	if len(r.user) > 0 && len(r.password) > 0 {
		req.SetBasicAuth(r.user, r.password)
	}

	req.Header.Set("Content-Type", "text/xml")

	return req, nil
}

func (r *XMLRPCClient) processResponse(resp *http.Response, processBody func(io.ReadCloser, error)) {
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		if r.verbose {
			fmt.Println("Bad Response:", resp.Status)
		}
		processBody(emptyReader, fmt.Errorf("Bad response with status code %d", resp.StatusCode))
	} else {
		processBody(resp.Body, nil)
	}
}

func (r *XMLRPCClient) postInetHTTP(method string, url string, data interface{}, processBody func(io.ReadCloser, error)) {
	req, err := r.createHTTPRequest(method, url, data)
	if err != nil {
		return
	}

	if r.timeout > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
		defer cancel()
		req = req.WithContext(ctx)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if r.verbose {
			fmt.Println("Fail to send request to supervisord:", err)
		}
		return
	}
	r.processResponse(resp, processBody)

}

func (r *XMLRPCClient) postUnixHTTP(method string, path string, data interface{}, processBody func(io.ReadCloser, error)) {
	var conn net.Conn
	var err error
	if r.timeout > 0 {
		conn, err = net.DialTimeout("unix", path, r.timeout)
	} else {
		conn, err = net.Dial("unix", path)
	}
	if err != nil {
		if r.verbose {
			fmt.Printf("Fail to connect unix socket path: %s\n", r.serverurl)
		}
		return
	}
	defer conn.Close()

	if r.timeout > 0 {
		if err := conn.SetDeadline(time.Now().Add(r.timeout)); err != nil {
			return
		}
	}
	req, err := r.createHTTPRequest(method, "/RPC2", data)

	if err != nil {
		return
	}
	err = req.Write(conn)
	if err != nil {
		if r.verbose {
			fmt.Printf("Fail to write to unix socket %s\n", r.serverurl)
		}
		return
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		if r.verbose {
			fmt.Printf("Fail to read response %s\n", err)
		}
		return
	}
	r.processResponse(resp, processBody)

}
func (r *XMLRPCClient) post(method string, data interface{}, processBody func(io.ReadCloser, error)) {
	url, err := url.Parse(r.serverurl)
	if err != nil {
		fmt.Printf("Malform url:%s\n", url)
		return
	}
	if url.Scheme == "http" || url.Scheme == "https" {
		r.postInetHTTP(method, r.URL(), data, processBody)
	} else if url.Scheme == "unix" {
		r.postUnixHTTP(method, url.Path, data, processBody)
	} else {
		fmt.Printf("Unsupported URL scheme:%s\n", url.Scheme)
	}

}

// GetVersion send get the supervisor http version request
func (r *XMLRPCClient) GetVersion() (reply VersionReply, err error) {
	ins := struct{}{}
	r.post("supervisor.getVersion", &ins, func(body io.ReadCloser, procError error) {
		err = procError
		if err == nil {
			err = xml.DecodeClientResponse(body, reply)
		}
	})
	return
}

// GetAllProcessInfo get all the processes of superisor
func (r *XMLRPCClient) GetAllProcessInfo() (reply AllProcessInfoReply, err error) {
	ins := struct{}{}
	r.post("supervisor.getAllProcessInfo", &ins, func(body io.ReadCloser, procError error) {
		err = procError
		if err == nil {
			err = xml.DecodeClientResponse(body, &reply)
		}
	})

	return
}

// ChangeProcessState change the proccess state
func (r *XMLRPCClient) ChangeProcessState(change string, processName string) (reply StartStopReply, err error) {
	if !(change == "start" || change == "stop") {
		err = fmt.Errorf("Incorrect required state")
		return
	}

	ins := struct{ Value string }{processName}
	r.post(fmt.Sprintf("supervisor.%sProcess", change), &ins, func(body io.ReadCloser, procError error) {
		err = procError
		if err == nil {
			err = xml.DecodeClientResponse(body, &reply)
		}
	})

	return
}

// ChangeAllProcessState change all the program to same state( start/stop )
func (r *XMLRPCClient) ChangeAllProcessState(change string) (reply AllProcessInfoReply, err error) {
	if !(change == "start" || change == "stop") {
		err = fmt.Errorf("Incorrect required state")
		return
	}
	ins := struct{ Wait bool }{true}
	r.post(fmt.Sprintf("supervisor.%sAllProcesses", change), &ins, func(body io.ReadCloser, procError error) {
		err = procError
		if err == nil {
			err = xml.DecodeClientResponse(body, &reply)
		}
	})
	return
}

// Shutdown shutdown the supervisor
func (r *XMLRPCClient) Shutdown() (reply ShutdownReply, err error) {
	ins := struct{}{}
	r.post("supervisor.shutdown", &ins, func(body io.ReadCloser, procError error) {
		err = procError
		if err == nil {
			err = xml.DecodeClientResponse(body, &reply)
		}

	})

	return
}

// ReloadConfig ask supervisor reload the configuration
func (r *XMLRPCClient) ReloadConfig() (reply types.ReloadConfigResult, err error) {
	ins := struct{}{}

	xmlProcMgr := NewXMLProcessorManager()
	reply.AddedGroup = make([]string, 0)
	reply.ChangedGroup = make([]string, 0)
	reply.RemovedGroup = make([]string, 0)
	i := -1
	hasValue := false
	xmlProcMgr.AddNonLeafProcessor("methodResponse/params/param/value/array/data", func() {
		if hasValue {
			hasValue = false
		} else {
			i++
		}
	})
	xmlProcMgr.AddLeafProcessor("methodResponse/params/param/value/array/data/value", func(value string) {
		hasValue = true
		i++
		switch i {
		case 0:
			reply.AddedGroup = append(reply.AddedGroup, value)
		case 1:
			reply.ChangedGroup = append(reply.ChangedGroup, value)
		case 2:
			reply.RemovedGroup = append(reply.RemovedGroup, value)
		}
	})
	r.post("supervisor.reloadConfig", &ins, func(body io.ReadCloser, procError error) {
		err = procError
		if err == nil {
			xmlProcMgr.ProcessXML(body)
		}
	})
	return
}

// SignalProcess send signal to program
func (r *XMLRPCClient) SignalProcess(signal string, name string) (reply types.BooleanReply, err error) {
	ins := types.ProcessSignal{Name: name, Signal: signal}
	r.post("supervisor.signalProcess", &ins, func(body io.ReadCloser, procError error) {
		err = procError
		if err == nil {
			err = xml.DecodeClientResponse(body, &reply)
		}
	})
	return
}

// SignalAll send signal to all the programs
func (r *XMLRPCClient) SignalAll(signal string) (reply AllProcessInfoReply, err error) {
	ins := struct{ Signal string }{signal}
	r.post("supervisor.signalProcess", &ins, func(body io.ReadCloser, procError error) {
		err = procError
		if err == nil {
			err = xml.DecodeClientResponse(body, &reply)
		}
	})

	return
}

// GetProcessInfo get the process information of one program
func (r *XMLRPCClient) GetProcessInfo(process string) (reply types.ProcessInfo, err error) {
	ins := struct{ Name string }{process}
	result := struct{ Reply types.ProcessInfo }{}
	r.post("supervisor.getProcessInfo", &ins, func(body io.ReadCloser, procError error) {
		err = procError
		if err == nil {
			err = xml.DecodeClientResponse(body, &result)
			if err == nil {
				reply = result.Reply
			} else if r.verbose {
				fmt.Printf("Fail to decode to types.ProcessInfo\n")
			}
		}
	})

	return
}
