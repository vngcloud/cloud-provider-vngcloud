package vngcloud

import (
	"github.com/vngcloud/vngcloud-go-sdk/client"
	lObjects "github.com/vngcloud/vngcloud-go-sdk/vngcloud/objects"
	"github.com/vngcloud/vngcloud-go-sdk/vngcloud/services/loadbalancer/v2/certificates"
	"k8s.io/klog/v2"
)

func ImportCertificate(client *client.ServiceClient, projectID string, opt *certificates.ImportOpts) (*lObjects.Certificate, error) {
	klog.V(5).Infoln("[API] ImportCertificate: ", "opt: ", opt)
	opt.ProjectID = projectID
	resp, err := certificates.Import(client, opt)
	klog.V(5).Infoln("[API] ImportCertificate: ", "resp: ", resp, "err: ", err)
	return resp, err
}

func ListCertificate(client *client.ServiceClient, projectID string) ([]*lObjects.Certificate, error) {
	// klog.V(5).Infoln("[API] ListCertificate: ")
	opt := &certificates.ListOpts{}
	opt.ProjectID = projectID
	resp, err := certificates.List(client, opt)
	// klog.V(5).Infoln("[API] ListCertificate: ", "resp: ", resp, "err: ", err)
	return resp, err
}

func GetCertificate(client *client.ServiceClient, projectID string, certificateID string) (*lObjects.Certificate, error) {
	klog.V(5).Infoln("[API] GetCertificate: ", "certificateID: ", certificateID)
	opt := &certificates.GetOpts{}
	opt.ProjectID = projectID
	opt.CaID = certificateID
	resp, err := certificates.Get(client, opt)
	klog.V(5).Infoln("[API] GetCertificate: ", "resp: ", resp, "err: ", err)
	return resp, err
}

func DeleteCertificate(client *client.ServiceClient, projectID string, certificateID string) error {
	klog.V(5).Infoln("[API] DeleteCertificate: ", "certificateID: ", certificateID)
	opt := &certificates.DeleteOpts{}
	opt.ProjectID = projectID
	opt.CaID = certificateID
	err := certificates.Delete(client, opt)
	klog.V(5).Infoln("[API] DeleteCertificate: ", "err: ", err)
	return err
}
