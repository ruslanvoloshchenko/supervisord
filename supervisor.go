package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
)

const (
	VERSION = "3.0"
)

type Supervisor struct {
	config     *Config
	procMgr    *ProcessManager
	xmlRPC     *XmlRPC
	logger     Logger
	restarting bool
}

type ProcessInfo struct {
	Name           string `xml:"name"`
	Group          string `xml:"group"`
	Description    string `xml:"description"`
	Start          int64  `xml:"start"`
	Stop           int64  `xml:"stop"`
	Now            int64  `xml:"now"`
	State          int    `xml:"state"`
	Statename      string `xml:"statename"`
	Spawnerr       string `xml:"spawnerr"`
	Exitstatus     int    `xml:"exitstatus"`
	Logfile        string `xml:"logfile"`
	Stdout_logfile string `xml:"stdout_logfile"`
	Stderr_logfile string `xml:"stderr_logfile"`
	Pid            int    `xml:"pid"`
}

type StartProcessArgs struct {
	Name string
	Wait bool `default:"true"`
}

type ProcessSignal struct {
	Name   string
	Signal string
}

type ProcessStdin struct {
	Name  string
	Chars string
}

type RemoteCommEvent struct {
	Type string
	Data string
}

type StateInfo struct {
	Statecode int    `xml:"statecode"`
	Statename string `xml:"statename"`
}

type RpcTaskResult struct {
	Name        string `xml:"name"`
	Group       string `xml:"group"`
	Status      int    `xml:"status"`
	Description string `xml:"description"`
}

type LogReadInfo struct {
	Offset int
	Length int
}

type ProcessLogReadInfo struct {
	Name   string
	Offset int
	Length int
}

type ProcessTailLog struct {
	LogData  string
	Offset   int64
	Overflow bool
}

func NewSupervisor(configFile string) *Supervisor {
	return &Supervisor{config: NewConfig(configFile),
		procMgr: newProcessManager(),
		xmlRPC:  NewXmlRPC()}
}

func (s *Supervisor) GetConfig() *Config {
	return s.config
}

func (s *Supervisor) GetVersion(r *http.Request, args *struct{}, reply *struct{ Version string }) error {
	reply.Version = VERSION
	return nil
}

func (s *Supervisor) GetSupervisorVersion(r *http.Request, args *struct{}, reply *struct{ Version string }) error {
	reply.Version = VERSION
	return nil
}

func (s *Supervisor) GetIdentification(r *http.Request, args *struct{}, reply *struct{ Id string }) error {
	reply.Id = s.GetSupervisorId()
	return nil
}

func (s *Supervisor) GetSupervisorId() string {
	entry, ok := s.config.GetSupervisord()
	if ok {
		return entry.GetString("identifier", "supervisor")
	} else {
		return "supervisor"
	}
}

func (s *Supervisor) GetState(r *http.Request, args *struct{}, reply *struct{ StateInfo StateInfo }) error {
	//statecode     statename
	//=======================
	// 2            FATAL
	// 1            RUNNING
	// 0            RESTARTING
	// -1           SHUTDOWN
	log.Debug("Get state")
	reply.StateInfo.Statecode = 1
	reply.StateInfo.Statename = "RUNNING"
	return nil
}

func (s *Supervisor) GetPID(r *http.Request, args *struct{}, reply *struct{ Pid int }) error {
	reply.Pid = os.Getpid()
	return nil
}

func (s *Supervisor) ReadLog(r *http.Request, args *LogReadInfo, reply *struct{ Log string }) error {
	data, err := s.logger.ReadLog(int64(args.Offset), int64(args.Length))
	reply.Log = data
	return err
}

func (s *Supervisor) ClearLog(r *http.Request, args *struct{}, reply *struct{ Ret bool }) error {
	err := s.logger.ClearAllLogFile()
	reply.Ret = err == nil
	return err
}

func (s *Supervisor) Shutdown(r *http.Request, args *struct{}, reply *struct{ Ret bool }) error {
	reply.Ret = true
	log.Info("received rpc request to stop all processes & exit")
	s.procMgr.StopAllProcesses()
	go func() {
		time.Sleep(1 * time.Second)
		os.Exit(0)
	}()
	return nil
}

func (s *Supervisor) Restart(r *http.Request, args *struct{}, reply *struct{ Ret bool }) error {
	log.Info("Receive instruction to restart")
	s.restarting = true
	reply.Ret = true
	return nil
}

func (s *Supervisor) IsRestarting() bool {
	return s.restarting
}

func getProcessInfo(proc *Process) *ProcessInfo {
	return &ProcessInfo{Name: proc.GetName(),
		Group:          proc.GetGroup(),
		Description:    proc.GetDescription(),
		Start:          proc.GetStartTime().Unix(),
		Stop:           proc.GetStopTime().Unix(),
		Now:            time.Now().Unix(),
		State:          int(proc.GetState()),
		Statename:      proc.GetState().String(),
		Spawnerr:       "",
		Exitstatus:     proc.GetExitstatus(),
		Logfile:        proc.GetStdoutLogfile(),
		Stdout_logfile: proc.GetStdoutLogfile(),
		Stderr_logfile: proc.GetStderrLogfile(),
		Pid:            proc.GetPid()}

}

func (s *Supervisor) GetAllProcessInfo(r *http.Request, args *struct{}, reply *struct{ AllProcessInfo []ProcessInfo }) error {
	reply.AllProcessInfo = make([]ProcessInfo, 0)
	s.procMgr.ForEachProcess(func(proc *Process) {
		procInfo := getProcessInfo(proc)
		reply.AllProcessInfo = append(reply.AllProcessInfo, *procInfo)
	})

	return nil
}

func (s *Supervisor) GetProcessInfo(r *http.Request, args *struct{ Name string }, reply *struct{ ProcInfo ProcessInfo }) error {
	log.Debug("Get process info of: ", args.Name)
	proc := s.procMgr.Find(args.Name)
	if proc == nil {
		return fmt.Errorf("no process named %s", args.Name)
	}

	reply.ProcInfo = *getProcessInfo(proc)
	return nil
}

func (s *Supervisor) StartProcess(r *http.Request, args *StartProcessArgs, reply *struct{ Success bool }) error {
	proc := s.procMgr.Find(args.Name)

	if proc == nil {
		return fmt.Errorf("fail to find process %s", args.Name)
	}
	proc.Start(args.Wait)
	reply.Success = true
	return nil
}

func (s *Supervisor) StartAllProcesses(r *http.Request, args *struct {
	Wait bool `default:"true"`
}, reply *struct{ RpcTaskResults []RpcTaskResult }) error {
	s.procMgr.ForEachProcess(func(proc *Process) {
		proc.Start(args.Wait)
		processInfo := *getProcessInfo(proc)
		reply.RpcTaskResults = append(reply.RpcTaskResults, RpcTaskResult{
			Name:        processInfo.Name,
			Group:       processInfo.Group,
			Status:      SUCCESS,
			Description: "OK",
		})
	})
	return nil
}

func (s *Supervisor) StartProcessGroup(r *http.Request, args *StartProcessArgs, reply *struct{ AllProcessInfo []ProcessInfo }) error {
	log.WithFields(log.Fields{"group": args.Name}).Info("start process group")
	s.procMgr.ForEachProcess(func(proc *Process) {
		if proc.GetGroup() == args.Name {
			proc.Start(args.Wait)
			reply.AllProcessInfo = append(reply.AllProcessInfo, *getProcessInfo(proc))
		}
	})

	return nil
}

func (s *Supervisor) StopProcess(r *http.Request, args *StartProcessArgs, reply *struct{ Success bool }) error {
	proc := s.procMgr.Find(args.Name)
	if proc == nil {
		return fmt.Errorf("fail to find process %s", args.Name)
	}
	proc.Stop(args.Wait)
	reply.Success = true
	return nil
}

func (s *Supervisor) StopProcessGroup(r *http.Request, args *StartProcessArgs, reply *struct{ AllProcessInfo []ProcessInfo }) error {
	log.WithFields(log.Fields{"group": args.Name}).Info("stop process group")
	s.procMgr.ForEachProcess(func(proc *Process) {
		if proc.GetGroup() == args.Name {
			proc.Stop(args.Wait)
			reply.AllProcessInfo = append(reply.AllProcessInfo, *getProcessInfo(proc))
		}
	})
	return nil
}

func (s *Supervisor) StopAllProcesses(r *http.Request, args *struct {
	Wait bool `default:"true"`
}, reply *struct{ RpcTaskResults []RpcTaskResult }) error {
	s.procMgr.ForEachProcess(func(proc *Process) {
		proc.Stop(args.Wait)
		processInfo := *getProcessInfo(proc)
		reply.RpcTaskResults = append(reply.RpcTaskResults, RpcTaskResult{
			Name:        processInfo.Name,
			Group:       processInfo.Group,
			Status:      SUCCESS,
			Description: "OK",
		})
	})
	return nil
}

func (s *Supervisor) SignalProcess(r *http.Request, args *ProcessSignal, reply *struct{ Success bool }) error {
	proc := s.procMgr.Find(args.Name)
	if proc == nil {
		return fmt.Errorf("No process named %s", args.Name)
	}
	proc.Signal(toSignal(args.Signal))
	return nil
}

func (s *Supervisor) SignalProcessGroup(r *http.Request, args *ProcessSignal, reply *struct{ AllProcessInfo []ProcessInfo }) error {
	s.procMgr.ForEachProcess(func(proc *Process) {
		if proc.GetGroup() == args.Name {
			proc.Signal(toSignal(args.Signal))
		}
	})

	s.procMgr.ForEachProcess(func(proc *Process) {
		if proc.GetGroup() == args.Name {
			reply.AllProcessInfo = append(reply.AllProcessInfo, *getProcessInfo(proc))
		}
	})
	return nil
}

func (s *Supervisor) SignalAllProcesses(r *http.Request, args *ProcessSignal, reply *struct{ AllProcessInfo []ProcessInfo }) error {
	s.procMgr.ForEachProcess(func(proc *Process) {
		proc.Signal(toSignal(args.Signal))
	})
	s.procMgr.ForEachProcess(func(proc *Process) {
		reply.AllProcessInfo = append(reply.AllProcessInfo, *getProcessInfo(proc))
	})
	return nil
}

func (s *Supervisor) SendProcessStdin(r *http.Request, args *ProcessStdin, reply *struct{ Success bool }) error {
	proc := s.procMgr.Find(args.Name)
	if proc == nil {
		log.WithFields(log.Fields{"program": args.Name}).Error("program does not exist")
		return fmt.Errorf("NOT_RUNNING")
	}
	if proc.GetState() != RUNNING {
		log.WithFields(log.Fields{"program": args.Name}).Error("program does not run")
		return fmt.Errorf("NOT_RUNNING")
	}
	err := proc.SendProcessStdin(args.Chars)
	if err == nil {
		reply.Success = true
	} else {
		reply.Success = false
	}
	return err
}

func (s *Supervisor) SendRemoteCommEvent(r *http.Request, args *RemoteCommEvent, reply *struct{ Success bool }) error {
	emitEvent(NewRemoteCommunicationEvent(args.Type, args.Data))
	reply.Success = true
	return nil
}

func (s *Supervisor) Reload() error {
	//get the previous loaded programs
	prevPrograms := s.config.GetProgramNames()

	err := s.config.Load()

	if err == nil {
		s.setSupervisordInfo()
		s.startEventListeners()
		s.restarting = false
		s.createPrograms(prevPrograms)
		s.startHttpServer()
		s.startAutoStartPrograms()
		for {
			if s.IsRestarting() {
				s.procMgr.StopAllProcesses()
				break
			}
			time.Sleep(10 * time.Second)
		}
	}
	return err

}

func (s *Supervisor) createPrograms(prevPrograms []string) {

	programs := s.config.GetProgramNames()
	for _, entry := range s.config.GetPrograms() {
		s.procMgr.CreateProcess(s.GetSupervisorId(), entry)
	}
	removedPrograms := sub(prevPrograms, programs)
	for _, p := range removedPrograms {
		s.procMgr.Remove(p)
	}
}

func (s *Supervisor) startAutoStartPrograms() {
	s.procMgr.StartAutoStartPrograms()
}

func (s *Supervisor) startEventListeners() {
	eventListeners := s.config.GetEventListeners()
	for _, entry := range eventListeners {
		s.procMgr.CreateProcess(s.GetSupervisorId(), entry)
	}

	if len(eventListeners) > 0 {
		time.Sleep(1 * time.Second)
	}
}

func (s *Supervisor) startHttpServer() {
	httpServerConfig, ok := s.config.GetInetHttpServer()
	if ok {
		addr := httpServerConfig.GetString("port", "")
		if addr != "" {
			go s.xmlRPC.StartInetHttpServer(httpServerConfig.GetString("username", ""), httpServerConfig.GetString("password", ""), addr, s)
		}
	}

	httpServerConfig, ok = s.config.GetUnixHttpServer()
	if ok {
		env := NewStringExpression("here", s.config.GetConfigFileDir())
		sockFile, err := env.Eval(httpServerConfig.GetString("file", "/tmp/supervisord.sock"))
		if err == nil {
			go s.xmlRPC.StartUnixHttpServer(httpServerConfig.GetString("username", ""), httpServerConfig.GetString("password", ""), sockFile, s)
		}
	}

}

func (s *Supervisor) setSupervisordInfo() {
	supervisordConf, ok := s.config.GetSupervisord()
	if ok {
		//set supervisord log

		env := NewStringExpression("here", s.config.GetConfigFileDir())
		logFile, err := env.Eval(supervisordConf.GetString("logfile", "supervisord.log"))
		s.logger = NewNullLogger()
		if err == nil {
			logfile_maxbytes := int64(supervisordConf.GetBytes("logfile_maxbytes", 50*1024*1024))
			logfile_backups := supervisordConf.GetInt("logfile_backups", 10)
			loglevel := supervisordConf.GetString("loglevel", "info")
			s.logger = NewFileLogger(logFile, logfile_maxbytes, logfile_backups, NewNullLogEventEmitter(), &sync.Mutex{})
			log.SetOutput(s.logger)
			log.SetLevel(toLogLevel(loglevel))
			log.SetFormatter(&log.TextFormatter{DisableColors: true})
		}
		//set the pid
		pidfile, err := env.Eval(supervisordConf.GetString("pidfile", "supervisord.pid"))
		if err == nil {
			f, err := os.Create(pidfile)
			if err == nil {
				fmt.Fprintf(f, "%d", os.Getpid())
				f.Close()
			}
		}
	}
}

func toLogLevel(level string) log.Level {
	switch strings.ToLower(level) {
	case "critical":
		return log.FatalLevel
	case "error":
		return log.ErrorLevel
	case "warn":
		return log.WarnLevel
	case "info":
		return log.InfoLevel
	default:
		return log.DebugLevel
	}
}

func sub(arr_1 []string, arr_2 []string) []string {
	result := make([]string, 0)
	for _, s := range arr_1 {
		exist := false
		for _, s2 := range arr_2 {
			if s == s2 {
				exist = true
			}
		}
		if !exist {
			result = append(result, s)
		}
	}
	return result
}

func (s *Supervisor) ReloadConfig(r *http.Request, args *struct{}, reply *struct{ Success bool }) error {
	err := s.Reload()
	if err == nil {
		reply.Success = true
	} else {
		reply.Success = false
	}
	return err
}

func (s *Supervisor) AddProcessGroup(r *http.Request, args *struct{ Name string }, reply *struct{ Success bool }) error {
	reply.Success = false
	return nil
}

func (s *Supervisor) RemoveProcessGroup(r *http.Request, args *struct{ Name string }, reply *struct{ Success bool }) error {
	reply.Success = false
	return nil
}

func (s *Supervisor) ReadProcessStdoutLog(r *http.Request, args *ProcessLogReadInfo, reply *struct{ LogData string }) error {
	proc := s.procMgr.Find(args.Name)
	if proc == nil {
		return fmt.Errorf("No such process %s", args.Name)
	}
	var err error
	reply.LogData, err = proc.stdoutLog.ReadLog(int64(args.Offset), int64(args.Length))
	return err
}

func (s *Supervisor) ReadProcessStderrLog(r *http.Request, args *ProcessLogReadInfo, reply *struct{ LogData string }) error {
	proc := s.procMgr.Find(args.Name)
	if proc == nil {
		return fmt.Errorf("No such process %s", args.Name)
	}
	var err error
	reply.LogData, err = proc.stderrLog.ReadLog(int64(args.Offset), int64(args.Length))
	return err
}

func (s *Supervisor) TailProcessStdoutLog(r *http.Request, args *ProcessLogReadInfo, reply *ProcessTailLog) error {
	proc := s.procMgr.Find(args.Name)
	if proc == nil {
		return fmt.Errorf("No such process %s", args.Name)
	}
	var err error
	reply.LogData, reply.Offset, reply.Overflow, err = proc.stdoutLog.ReadTailLog(int64(args.Offset), int64(args.Length))
	return err
}

func (s *Supervisor) ClearProcessLogs(r *http.Request, args *struct{ Name string }, reply *struct{ Success bool }) error {
	proc := s.procMgr.Find(args.Name)
	if proc == nil {
		return fmt.Errorf("No such process %s", args.Name)
	}
	err1 := proc.stdoutLog.ClearAllLogFile()
	err2 := proc.stderrLog.ClearAllLogFile()
	reply.Success = err1 == nil && err2 == nil
	if err1 != nil {
		return err1
	}
	return err2
}

func (s *Supervisor) ClearAllProcessLogs(r *http.Request, args *struct{}, reply *struct{ RpcTaskResults []RpcTaskResult }) error {

	s.procMgr.ForEachProcess(func(proc *Process) {
		proc.stdoutLog.ClearAllLogFile()
		proc.stderrLog.ClearAllLogFile()
		procInfo := getProcessInfo(proc)
		reply.RpcTaskResults = append(reply.RpcTaskResults, RpcTaskResult{
			Name:        procInfo.Name,
			Group:       procInfo.Group,
			Status:      SUCCESS,
			Description: "OK",
		})
	})

	return nil
}
