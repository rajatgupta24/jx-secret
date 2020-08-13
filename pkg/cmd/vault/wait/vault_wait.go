package wait

import (
	"fmt"
	"time"

	"github.com/jenkins-x/jx-helpers/pkg/cmdrunner"
	"github.com/jenkins-x/jx-helpers/pkg/cobras/helper"
	"github.com/jenkins-x/jx-helpers/pkg/cobras/templates"
	"github.com/jenkins-x/jx-helpers/pkg/kube"
	"github.com/jenkins-x/jx-helpers/pkg/kube/pods"
	"github.com/jenkins-x/jx-helpers/pkg/termcolor"
	"github.com/jenkins-x/jx-logging/pkg/log"
	"github.com/jenkins-x/jx-secret/pkg/extsecrets/editor/vault"
	"github.com/jenkins-x/jx-secret/pkg/rootcmd"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"
)

var (
	cmdLong = templates.LongDesc(`
		Waits for vault to be ready for use
`)

	cmdExample = templates.Examples(`
		%s vault wait
	`)
)

// Options the options for the command
type Options struct {
	PodName       string
	Namespace     string
	WaitDuration  time.Duration
	PollPeriod    time.Duration
	NoEditorWait  bool
	CommandRunner cmdrunner.CommandRunner
	KubeClient    kubernetes.Interface
	Start         time.Time
}

// NewCmdWait creates a command object for the command
func NewCmdWait() (*cobra.Command, *Options) {
	o := &Options{}

	cmd := &cobra.Command{
		Use:     "wait",
		Short:   "Waits for vault to be ready for use",
		Long:    cmdLong,
		Example: fmt.Sprintf(cmdExample, rootcmd.BinaryName),
		Run: func(cmd *cobra.Command, args []string) {
			err := o.Run()
			helper.CheckErr(err)
		},
	}
	o.AddFlags(cmd)
	return cmd, o
}

// AddFlags adds the options flags to the command
func (o *Options) AddFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&o.PodName, "pod", "p", "vault-0", "the name of the vault pod which needs to be running before the port forward can take place")
	cmd.Flags().StringVarP(&o.Namespace, "ns", "n", "vault-infra", "the namespace where vault is running")
	cmd.Flags().DurationVarP(&o.WaitDuration, "duration", "d", 5*time.Minute, "the maximum time period to wait for vault to be ready")
	cmd.Flags().DurationVarP(&o.PollPeriod, "poll", "", 2*time.Second, "the polling period to check if the secrets are valid")
}

// Run implements the command
func (o *Options) Run() error {
	o.Start = time.Now()
	var err error
	o.KubeClient, o.Namespace, err = kube.LazyCreateKubeClientAndNamespace(o.KubeClient, o.Namespace)
	if err != nil {
		return errors.Wrap(err, "failed to create kube client")
	}
	err = o.waitForPod()
	if err != nil {
		return errors.Wrapf(err, "failed to wait for vault pod")
	}
	if o.NoEditorWait {
		return nil
	}
	return o.waitForEditor()
}

func (o *Options) waitForPod() error {
	log.Logger().Infof("waiting for vault pod %s in namespace %s to be ready...", termcolor.ColorInfo(o.PodName), termcolor.ColorInfo(o.Namespace))
	err := pods.WaitForPodNameToBeReady(o.KubeClient, o.Namespace, o.PodName, o.WaitDuration)
	if err != nil {
		return errors.Wrapf(err, "failed to wait for pod %s to be ready in namespace %s", o.PodName, o.Namespace)
	}
	log.Logger().Infof("pod %s in namespace %s is ready", termcolor.ColorInfo(o.PodName), termcolor.ColorInfo(o.Namespace))
	return nil
}

func (o *Options) waitForEditor() error {
	endTime := o.Start.Add(o.WaitDuration)

	for {
		_, err := vault.NewEditor(o.CommandRunner, o.KubeClient)
		if err == nil {
			log.Logger().Infof("managed to verify we can connect to vault")
			return nil
		}
		if err != nil {
			log.Logger().Warnf("could not connect to vault: %s", err.Error())
		}

		if time.Now().After(endTime) {
			return errors.Errorf("timed out waiting %s for vault to be ready", o.WaitDuration.String())
		}
		time.Sleep(o.PollPeriod)
	}
}
