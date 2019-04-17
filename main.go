package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/alecthomas/kingpin"
	"github.com/oracle/oci-go-sdk/common"
	"github.com/oracle/oci-go-sdk/core"
	"github.com/oracle/oci-go-sdk/identity"
	"github.com/sirupsen/logrus"
)

var (
	configfile string
	debug      bool
	outfile    = "/etc/hosts"
)

func main() {
	compartment := ""
	hosts := bytes.NewBuffer([]byte{})

	app := kingpin.New("oci-hosts-builder", "Builds an /etc/hosts style file for a Oracle Cloud tenancy that spans multiple VCN")
	app.Arg("compartment", "The compartment OCID to query, specify the root compartment to traverse all compartments").Required().StringVar(&compartment)
	app.Arg("hosts", "The file to append the discovered nodes to").Default("/etc/hosts").StringVar(&outfile)
	app.Flag("config", "OCI Configuration file with paths to access keys etc").StringVar(&configfile)
	app.Flag("debug", "Enable debug logging").Default("false").BoolVar(&debug)

	kingpin.MustParse(app.Parse(os.Args[1:]))

	logrus.SetLevel(logrus.InfoLevel)
	if debug {
		logrus.SetLevel(logrus.DebugLevel)
	}

	cnt := 0

	err := compartments(compartment, func(compartment *string) {
		logrus.Debugf("Processing compartment %s", *compartment)

		err := vcns(compartment, func(vcn core.Vcn) {
			logrus.Debugf("Processing VCN %s (%s)", *vcn.DnsLabel, *vcn.Id)
			err := subnets(compartment, vcn.Id, func(subnet core.Subnet) {
				logrus.Debugf("Processing Subnet %s (%s)", *subnet.DnsLabel, *subnet.Id)
				err := privateIPs(subnet.Id, func(private core.PrivateIp) {
					logrus.Debugf("Processing Private IP %s", *private.IpAddress)
					if *private.IsPrimary {
						if private.HostnameLabel != nil && private.IpAddress != nil {
							cnt++
							fmt.Fprintf(hosts, "%-20s%s.%s.%s.oraclevcn.com %s.%s %s\n", *private.IpAddress, *private.HostnameLabel, *subnet.DnsLabel, *vcn.DnsLabel, *private.HostnameLabel, *subnet.DnsLabel, *private.HostnameLabel)
						}
					}
				})
				if err != nil {
					logrus.Errorf("Could not retrieve private ips: %s", err.Error())
				}
			})
			if err != nil {
				logrus.Errorf("Could not retrieve subnets: %s", err.Error())
			}
		})
		if err != nil {
			logrus.Errorf("Could not list VCNs in compartment %s: %s", *compartment, err.Error())
			return
		}

	})
	kingpin.FatalIfError(err, "Could not retrieve compartments")

	err = writeHosts(outfile, hosts)
	kingpin.FatalIfError(err, "Could not write %s", outfile)
	logrus.Infof("Wrote %d lines to %s", cnt, outfile)
}

func ociConfig() (common.ConfigurationProvider, error) {
	_, err := os.Stat(configfile)
	if err != nil {
		return common.DefaultConfigProvider(), nil
	}

	return common.ConfigurationProviderFromFile(configfile, "")
}

func writeHosts(target string, hosts *bytes.Buffer) error {
	tmpfile, err := ioutil.TempFile("", "hosts")
	if err != nil {
		return err
	}
	defer os.Remove(tmpfile.Name())

	tfile, err := os.Open(target)
	if err != nil {
		return err
	}
	defer tfile.Close()

	wrote := false

	scanner := bufio.NewScanner(tfile)
	for scanner.Scan() {
		line := scanner.Text()

		if strings.Contains(line, "# oci_hosts") {
			fmt.Fprintf(tmpfile, "# oci_hosts text below this will be removed\n")
			fmt.Fprintf(tmpfile, hosts.String())
			wrote = true
			break
		}

		fmt.Fprintf(tmpfile, "%s\n", line)
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	// append if we did not find a token, this ensure it works on a new file and does not corrupt existing ones
	if !wrote {
		fmt.Fprintf(tmpfile, "# oci_hosts text below this will be removed\n")
		fmt.Fprintf(tmpfile, hosts.String())
	}

	err = os.Chmod(target, 0644)
	if err != nil {
		return err
	}

	err = os.Rename(tmpfile.Name(), target)
	if err != nil {
		return err
	}

	return nil
}

func privateIPs(subnet *string, f func(ip core.PrivateIp)) error {
	configProvider, err := ociConfig()
	if err != nil {
		return err
	}

	vnClient, err := core.NewVirtualNetworkClientWithConfigurationProvider(configProvider)
	if err != nil {
		return err
	}

	req := core.ListPrivateIpsRequest{
		SubnetId: subnet,
	}

	for {
		privates, err := vnClient.ListPrivateIps(context.Background(), req)
		if err != nil {
			return err
		}

		for _, ip := range privates.Items {
			f(ip)
		}

		if privates.OpcNextPage != nil {
			req.Page = privates.OpcNextPage
		} else {
			break
		}
	}

	return nil
}

func compartments(compartment string, f func(compartmentId *string)) error {
	configProvider, err := ociConfig()
	if err != nil {
		return err
	}

	subtree := common.Bool(false)
	if strings.Contains(compartment, "tenancy") {
		subtree = common.Bool(true)
	}

	idClient, err := identity.NewIdentityClientWithConfigurationProvider(configProvider)
	if err != nil {
		return err
	}

	compartments, err := idClient.ListCompartments(context.Background(), identity.ListCompartmentsRequest{
		CompartmentId:          &compartment,
		CompartmentIdInSubtree: subtree,
		Limit: common.Int(100),
	})
	if err != nil {
		return err
	}

	if len(compartments.Items) == 0 {
		f(&compartment)
		return nil
	}

	for _, compartment := range compartments.Items {
		f(compartment.Id)
	}

	return nil
}

func vcns(compartment *string, f func(vcn core.Vcn)) error {
	configProvider, err := ociConfig()
	if err != nil {
		return err
	}

	vnClient, err := core.NewVirtualNetworkClientWithConfigurationProvider(configProvider)
	if err != nil {
		return err
	}

	req := core.ListVcnsRequest{
		CompartmentId: compartment,
	}

	for {
		vcns, err := vnClient.ListVcns(context.Background(), req)
		if err != nil {
			return err
		}

		for _, vcn := range vcns.Items {
			f(vcn)
		}

		if vcns.OpcNextPage != nil {
			req.Page = vcns.OpcNextPage
		} else {
			break
		}
	}

	return nil
}

func subnets(compartment *string, vcn *string, f func(s core.Subnet)) error {
	configProvider, err := ociConfig()
	if err != nil {
		return err
	}

	vnClient, err := core.NewVirtualNetworkClientWithConfigurationProvider(configProvider)
	if err != nil {
		return err
	}

	req := core.ListSubnetsRequest{
		CompartmentId: compartment,
		VcnId:         vcn,
	}

	for {
		subnets, err := vnClient.ListSubnets(context.Background(), req)
		if err != nil {
			return err
		}

		for _, subnet := range subnets.Items {
			f(subnet)
		}

		if subnets.OpcNextPage != nil {
			req.Page = subnets.OpcNextPage
		} else {
			break
		}
	}

	return nil
}
