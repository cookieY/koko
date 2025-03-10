package srvconn

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jumpserver/koko/pkg/config"
	"github.com/jumpserver/koko/pkg/localcommand"
	"github.com/jumpserver/koko/pkg/logger"
	"github.com/jumpserver/koko/pkg/utils"
)

var (
	InValidToken = errors.New("invalid token")

	_ ServerConnection = (*K8sCon)(nil)
)

const (
	k8sInitFilename = "init-kubectl.sh"

	checkTokenCommand = `kubectl --insecure-skip-tls-verify=%s --token=%s --server=%s auth can-i get pods`
)

func isValidK8sUserToken(o *k8sOptions) bool {
	skipVerifyTls := "true"
	token := o.Token
	server := o.ClusterServer
	if !o.IsSkipTls {
		skipVerifyTls = "false"
	}
	c := exec.Command("bash", "-c",
		fmt.Sprintf(checkTokenCommand, skipVerifyTls, token, server))
	out, err := c.CombinedOutput()
	if err != nil {
		logger.Info(err)
	}
	result := strings.TrimSpace(string(out))
	switch strings.ToLower(result) {
	case "yes", "no":
		logger.Info("K8sCon check token success")
		return true
	}
	logger.Errorf("K8sCon check token err: %s", result)
	return false
}

func NewK8sCon(ops ...k8sOption) *K8sCon {
	args := &k8sOptions{
		Username:      os.Getenv("USER"),
		ClusterServer: "https://127.0.0.1:8443",
		Token:         "",
		IsSkipTls:     true,
		ExtraEnv:      map[string]string{},
	}
	for _, setter := range ops {
		setter(args)
	}
	return &K8sCon{options: args}
}

type K8sCon struct {
	options *k8sOptions
	*localcommand.LocalCommand
}

func (k *K8sCon) Connect(win Windows) (err error) {
	if !isValidK8sUserToken(k.options) {
		return InValidToken
	}
	lcmd, err := startK8SLocalCommand(k)
	if err != nil {
		logger.Errorf("K8sCon start local pty err: %s", err)
		return fmt.Errorf("K8sCon start local pty err: %w", err)
	}
	_ = lcmd.SetWinSize(win.Width, win.Height)
	k.LocalCommand = lcmd
	logger.Infof("Connect K8s cluster server %s success", k.options.ClusterServer)
	return
}

func (k *K8sCon) KeepAlive() error {
	return nil
}

type k8sOptions struct {
	ClusterServer string // https://172.16.10.51:8443
	Username      string // user 系统用户名
	Token         string // 授权token
	IsSkipTls     bool
	ExtraEnv      map[string]string
}

func (o *k8sOptions) Env() []string {
	token, err := utils.Encrypt(o.Token, config.CipherKey)
	if err != nil {
		logger.Errorf("Encrypt k8s token err: %s", err)
		token = o.Token
	}
	skipTls := "true"
	if !o.IsSkipTls {
		skipTls = "false"
	}
	return []string{
		fmt.Sprintf("KUBECTL_USER=%s", o.Username),
		fmt.Sprintf("KUBECTL_CLUSTER=%s", o.ClusterServer),
		fmt.Sprintf("KUBECTL_INSECURE_SKIP_TLS_VERIFY=%s", skipTls),
		fmt.Sprintf("K8S_ENCRYPTED_TOKEN=%s", token),
		fmt.Sprintf("WELCOME_BANNER=%s", config.KubectlBanner),
	}
}

func startK8SLocalCommand(con *K8sCon) (*localcommand.LocalCommand, error) {
	pwd, _ := os.Getwd()
	shPath := filepath.Join(pwd, k8sInitFilename)
	argv := []string{
		"--fork",
		"--pid",
		"--mount-proc",
		shPath,
	}
	return localcommand.New("unshare", argv, localcommand.WithEnv(con.options.Env()))
}

type k8sOption func(*k8sOptions)

func K8sUsername(username string) k8sOption {
	return func(args *k8sOptions) {
		args.Username = username
	}
}

func K8sToken(token string) k8sOption {
	return func(args *k8sOptions) {
		args.Token = token
	}
}

func K8sClusterServer(clusterServer string) k8sOption {
	return func(args *k8sOptions) {
		args.ClusterServer = clusterServer
	}
}

func K8sExtraEnvs(envs map[string]string) k8sOption {
	return func(args *k8sOptions) {
		args.ExtraEnv = envs
	}
}

func K8sSkipTls(isSkipTls bool) k8sOption {
	return func(args *k8sOptions) {
		args.IsSkipTls = isSkipTls
	}
}
