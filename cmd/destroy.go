package cmd

import (
	"bytes"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
)

func (cl *ClusterData) DestroyApiDnsRecordsOsp(v *OpenshiftData, a *AuthData, wg *sync.WaitGroup) error {
	c, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}

	var infra_id string
	if Username != "" {
		infra_id = Username + "-" + cl.ClusterName
	} else {
		infra_id = c.Username + "-" + cl.ClusterName
	}

	log.Infof("Destroying api DNS records for %s platform: %s.", cl.ClusterName, cl.Platform.Name)
	cmdName := "./bin/terraform"
	cmdArgs := []string{
		"destroy", "-target", "module." + cl.ClusterName + "-osp-dns",
		"-var", "infra_id=" + infra_id,
		"-var", "dns_domain=" + cl.DNSDomain,
		"-var", "public_network_name=" + cl.Platform.ExternalNetwork,
		"-var", "osp_auth_url=" + a.OpenStack.AuthURL,
		"-var", "osp_user_name=" + a.OpenStack.Username,
		"-var", "osp_user_password=" + a.OpenStack.Password,
		"-var", "osp_user_domain_name=" + a.OpenStack.UserDomainName,
		"-var", "osp_tenant_id=" + a.OpenStack.ProjectID,
		"-var", "osp_tenant_name=" + a.OpenStack.ProjectName,
		"-var", "osp_region=" + cl.Platform.Region,
		"-state", "tf/state/" + "terraform-" + cl.ClusterName + "-osp-dns.tfstate",
		"-auto-approve",
	}

	currentDir, err := os.Getwd()
	if err != nil {
		return errors.Wrapf(err, "%s", cl.ClusterName)
	}

	logFile := filepath.Join(currentDir, ".config", cl.ClusterName, ".openshift_install.log")
	f, err := os.OpenFile(logFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("Error opening file: %v", err)
	}

	defer f.Close()
	buf := &bytes.Buffer{}
	mwriter := io.MultiWriter(f, buf)

	cmd := exec.Command(cmdName, cmdArgs...)
	cmd.Stdout = mwriter
	cmd.Stderr = mwriter

	err = cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "Error waiting for resources destruction: %s\n%s", cl.ClusterName, buf.String())
	}

	log.WithFields(log.Fields{
		"cluster": cl.ClusterName,
	}).Debugf("%s %s", cl.ClusterName, buf.String())

	output := strings.Split(buf.String(), "\n")
	log.Infof("??? DNS records were destroyed for %s: %s", cl.ClusterName, output[len(output)-2])
	wg.Done()
	return nil
}

func (cl *ClusterData) DestroyAppsDnsRecordsOsp(a *AuthData, wg *sync.WaitGroup) error {
	infraDetails, err := cl.ExtractInfraDetails()
	if err != nil {
		return errors.Wrapf(err, "%s", infraDetails[0])
	}

	log.Infof("Destroying vxlan security group rules and apps DNS records for %s platform: %s.", cl.ClusterName, cl.Platform.Name)
	cmdName := "./bin/terraform"
	cmdArgs := []string{
		"destroy", "-target", "module." + cl.ClusterName + "-osp-sg",
		"-var", "infra_id=" + infraDetails[0],
		"-var", "dns_domain=" + cl.DNSDomain,
		"-var", "osp_auth_url=" + a.OpenStack.AuthURL,
		"-var", "osp_user_name=" + a.OpenStack.Username,
		"-var", "osp_user_password=" + a.OpenStack.Password,
		"-var", "osp_user_domain_name=" + a.OpenStack.UserDomainName,
		"-var", "osp_tenant_id=" + a.OpenStack.ProjectID,
		"-var", "osp_tenant_name=" + a.OpenStack.ProjectName,
		"-var", "osp_region=" + cl.Platform.Region,
		"-var", "public_network_name=" + cl.Platform.ExternalNetwork,
		"-state", "tf/state/" + "terraform-" + cl.ClusterName + "-osp-sg.tfstate",
		"-auto-approve",
	}

	currentDir, err := os.Getwd()
	if err != nil {
		return errors.Wrapf(err, "%s", cl.ClusterName)
	}

	logFile := filepath.Join(currentDir, ".config", cl.ClusterName, ".openshift_install.log")
	f, err := os.OpenFile(logFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("Error opening file: %v", err)
	}

	defer f.Close()
	buf := &bytes.Buffer{}
	mwriter := io.MultiWriter(f, buf)

	cmd := exec.Command(cmdName, cmdArgs...)
	cmd.Stdout = mwriter
	cmd.Stderr = mwriter

	err = cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "Error waiting for resources deletion: %s\n%s", cl.ClusterName, buf.String())
	}

	log.WithFields(log.Fields{
		"cluster": cl.ClusterName,
	}).Debugf("%s %s", cl.ClusterName, buf.String())

	output := strings.Split(buf.String(), "\n")
	log.Infof("??? Security group rules and DNS records were destr	oyed for %s: %s", cl.ClusterName, output[len(output)-2])
	wg.Done()
	return nil
}

func (cl *ClusterData) DestroyCluster(wg *sync.WaitGroup) error {
	log.Infof("Deleting resources for %s. Please be patient. Up to 45 minutes...", cl.ClusterName)
	currentDir, _ := os.Getwd()
	configDir := filepath.Join(currentDir, ".config", cl.ClusterName)
	cmdName := "./bin/openshift-install"
	cmdArgs := []string{"destroy", "cluster", "--dir", configDir, "--log-level", "debug"}

	cmd := exec.Command(cmdName, cmdArgs...)
	buf := &bytes.Buffer{}
	cmd.Stdout = buf
	cmd.Stderr = buf

	err := cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "Error waiting for deletion: %s\n%s", cl.ClusterName, buf.String())
	}

	glob := "terraform-" + cl.ClusterName + "-*"
	files, err := filepath.Glob(filepath.Join("tf", "state", glob))
	if err != nil {
		return err
	}

	for _, f := range files {
		log.Debugf("Removing %s", f)
		if err := os.Remove(f); err != nil {
			return err
		}
	}

	glob = "*-submariner-gw-machine-set.yaml"
	files, err = filepath.Glob(filepath.Join(".config", cl.ClusterName, glob))
	if err != nil {
		return err
	}

	for _, f := range files {
		log.Debugf("Removing %s", f)
		if err := os.Remove(f); err != nil {
			return err
		}
	}

	err = os.RemoveAll(filepath.Join(currentDir, ".config", cl.ClusterName))
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"cluster": cl.ClusterName,
	}).Debugf("%s %s", cl.ClusterName, buf.String())
	log.Infof("??? Resources for %s were removed.", cl.ClusterName)
	wg.Done()
	return nil
}

var destroyClustersCmd = &cobra.Command{
	Use:   "clusters",
	Short: "Destroy cluster resources",
	Run: func(cmd *cobra.Command, args []string) {

		if Debug {
			log.SetReportCaller(true)
			log.SetLevel(log.DebugLevel)
		}

		config, err := ParseConfigFile()
		if err != nil {
			log.Fatal(err)
		}

		err = GetDependencies(&config.Openshift)
		if err != nil {
			log.Fatal(err)
		}

		var openstackcls []ClusterData
		for _, cl := range config.Clusters {
			switch cl.Platform.Name {
			case "openstack":
				openstackcls = append(openstackcls, cl)
			}
		}

		var wg sync.WaitGroup
		wg.Add(len(openstackcls))
		for _, cl := range openstackcls {
			go func(cl ClusterData) {
				err := cl.DestroyApiDnsRecordsOsp(&config.Openshift, &config.Authentication, &wg)
				if err != nil {
					defer wg.Done()
					log.Error(err)
				}
			}(cl)
		}
		wg.Wait()

		wg.Add(len(openstackcls))
		for _, cl := range openstackcls {
			go func(cl ClusterData) {
				err := cl.DestroyAppsDnsRecordsOsp(&config.Authentication, &wg)
				if err != nil {
					defer wg.Done()
					log.Error(err)
				}
			}(cl)
		}
		wg.Wait()

		wg.Add(len(config.Clusters))
		for _, cl := range config.Clusters {
			go func(cl ClusterData) {
				err := cl.DestroyCluster(&wg)
				if err != nil {
					defer wg.Done()
					log.Fatal(err)
				}
			}(cl)
		}
		wg.Wait()
	},
}

func init() {
	var destroyCmd = &cobra.Command{Use: "destroy", Short: "Destroy resources"}
	rootCmd.AddCommand(destroyCmd)
	destroyCmd.AddCommand(destroyClustersCmd)
}
