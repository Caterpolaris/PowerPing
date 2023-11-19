package main

import (
	"fmt"
	"golang.org/x/crypto/ssh"
	"gopkg.in/yaml.v3"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type Config struct {
	PingParams PingParams   `yaml:"ping_params"`
	WaitTime   string       `yaml:"wait_time"`
	HostParams []HostParams `yaml:"host_params"`
}

type PingParams struct {
	IntervalTime string   `yaml:"interval_time"`
	RetryCount   int      `yaml:"retry_count"`
	Timeout      string   `yaml:"timeout"`
	TargetIPs    []string `yaml:"target_ips"`
}

type HostParams struct {
	HostIP   string `yaml:"host"`
	HostPort int    `yaml:"port"`
	HostName string `yaml:"hostname"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

func main() {
	executablePath, err := os.Executable()
	if err != nil {
		fmt.Println("无法获取程序执行路径：", err)
		return
	}
	configFilePath := filepath.Join(filepath.Dir(executablePath), "config.yml")
	logFilePath := filepath.Join(filepath.Dir(executablePath), "log.txt")
	initConfig(configFilePath)
	logFile, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("无法创建日志文件：%v", err)
	}
	defer logFile.Close()
	log.SetOutput(io.MultiWriter(logFile, os.Stderr))

	data, err := os.ReadFile(configFilePath)
	if err != nil {
		log.Fatalf("无法读取配置文件：%v", err)
	}

	// 解析配置文件
	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		log.Fatalf("无法解析配置文件：%v", err)
	}
	waitTime, err := time.ParseDuration(config.WaitTime)
	if err != nil {
		log.Fatalf("无法解析 WaitTime：%v", err)
	}
	intervalTimeTmp, err := time.ParseDuration(config.PingParams.IntervalTime)
	if err != nil {
		log.Fatalf("无法解析 IntervalTime：%v", err)
	}
	TimeoutTmp, err := time.ParseDuration(config.PingParams.Timeout)
	if err != nil {
		log.Fatalf("无法解析 IntervalTime：%v", err)
	}

	timeOut := int(TimeoutTmp.Seconds())

	fmt.Println(`启动成功.`)
	fmt.Println(`执行参数:`)
	fmt.Printf("判定间隔: %s\n", config.PingParams.IntervalTime)
	fmt.Printf("判定超时: %s\n", config.PingParams.Timeout)
	fmt.Printf("重试次数: %d\n", config.PingParams.RetryCount)
	fmt.Printf("关机等待时间: %s\n", config.WaitTime)
	fmt.Println("开始监控.")

	// 每隔intervalTimeTmp执行一次 Ping
	ticker := time.NewTicker(intervalTimeTmp)
	defer ticker.Stop()
	for {
		for range ticker.C {
			if !pingIP(config.PingParams.TargetIPs, timeOut, config.PingParams.RetryCount) {
				log.Printf(`所有IP无法Ping通,疑似断电,等待%s后关机...`, config.WaitTime)
				time.Sleep(waitTime)
				log.Printf(`关机前重新检测是否电力恢复.`)
				if pingIP(config.PingParams.TargetIPs, timeOut, config.PingParams.RetryCount) {
					log.Printf(`电力恢复,重新开始监控`)
				} else {
					log.Printf(`电力未恢复,开始关机`)
					hostAction(config.HostParams)
					log.Printf("本机关机")
					colseSelf()
				}
			}
		}
	}
}

// Ping IP 函数
func pingIP(targetIPs []string, timeout, retryCount int) bool {
	for _, ip := range targetIPs {
		for i := 0; i < retryCount; i++ {
			//cmd := exec.Command("ping", "-c", "1", "-W", fmt.Sprintf("%d", timeout), ip)
			var cmd *exec.Cmd
			switch runtime.GOOS {
			case "linux":
				cmd = exec.Command("ping", "-c", "1", "-W", fmt.Sprintf("%d", timeout), ip)
			case "windows":
				cmd = exec.Command("ping", "-n", "1", "-w", fmt.Sprintf("%d", timeout*1000), ip)
			case "darwin":
				cmd = exec.Command("ping", "-c", "1", "-W", fmt.Sprintf("%d", timeout), ip)
			default:
				fmt.Println("此系统暂不支持")
				os.Exit(0)
			}
			output, err := cmd.CombinedOutput()
			if err == nil {
				if strings.Contains(strings.ToLower(string(output)), "ttl") {
					return true
				}
			}

			log.Printf("尝试 Ping IP: %s，失败次数: %d\n", ip, i+1)
		}
	}

	return false
}

func hostAction(Hosts []HostParams) {
	for _, params := range Hosts {
		// 创建 SSH 客户端配置
		config := &ssh.ClientConfig{
			User: params.Username,
			Auth: []ssh.AuthMethod{
				ssh.Password(params.Password),
			},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		}

		// 构建 SSH 连接地址
		addr := fmt.Sprintf("%s:%d", params.HostIP, params.HostPort)

		// 连接 SSH 服务器
		client, err := ssh.Dial("tcp", addr, config)
		if err != nil {
			log.Printf("无法连接到 %s：%v", addr, err)
			continue
		}
		defer client.Close()

		// 执行关机命令
		session, err := client.NewSession()
		if err != nil {
			log.Printf("无法创建会话：%v", err)
			continue
		}
		defer session.Close()

		// 使用nohup解决立即关机导致的无返回超时问题
		command := "nohup /bin/bash -c 'sleep 3;shutdown now' >/dev/null 2>&1 &"

		err = session.Run(command)
		if err != nil {
			log.Printf("在 %s 上执行命令失败：%v", addr, err)
		} else {
			log.Printf("在 %s 上执行命令成功,3秒后关机", addr)
		}
	}
}

func colseSelf() {
	var cmd *exec.Cmd
	switch sysOs := runtime.GOOS; sysOs {
	case "windows":
		cmd = exec.Command("shutdown", "/s", "/t", "0")
	case "linux":
		cmd = exec.Command("shutdown", "-P", "now")
	case "darwin":
		cmd = exec.Command("shutdown", "-h", "now")
	default:
		log.Println(
			"此系统暂不支持关机:",
			sysOs,
		)
		return
	}

	err := cmd.Run()
	if err != nil {
		log.Println("执行本机关机失败 shutdown:", err)
		return
	}
}

// 创建配置文件
func initConfig(configFile string) {
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		config := Config{
			PingParams: PingParams{
				IntervalTime: "30s",
				RetryCount:   1,
				Timeout:      "1s",
				TargetIPs:    []string{"8.8.8.8"},
			},
			WaitTime: "5m",
			HostParams: []HostParams{
				{
					HostIP:   "192.168.1.1",
					HostPort: 22,
					HostName: "centos7",
					Username: "root",
					Password: "password",
				},
			},
		}

		// 将配置对象转换为 YAML 格式
		data, err := yaml.Marshal(&config)
		if err != nil {
			log.Fatalf("无法转换为 YAML 格式：%v", err)
		}

		// 创建文件并写入配置内容
		err = os.WriteFile(configFile, data, 0644)
		if err != nil {
			log.Fatalf("无法创建文件：%v", err)
		}

		fmt.Println("已创建", configFile, "并写入默认配置.")
		os.Exit(0)
	}
}
