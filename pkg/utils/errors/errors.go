package errors

import (
	vErr "errors"
	"fmt"

	vconError "github.com/vngcloud/vngcloud-go-sdk/error"
)

// ********************************************** ErrNodeAddressNotFound **********************************************

func NewErrNodeAddressNotFound(pNodeName, pInfo string) vconError.IErrorBuilder {
	err := new(ErrNodeAddressNotFound)
	err.NodeName = pNodeName
	if pInfo != "" {
		err.Info = pInfo
	}
	return err
}

func IsErrNodeAddressNotFound(pErr error) bool {
	_, ok := pErr.(*ErrNodeAddressNotFound)
	return ok
}

func (s *ErrNodeAddressNotFound) Error() string {
	s.DefaultError = fmt.Sprintf("can not find address of node [NodeName: %s]", s.NodeName)
	return s.ChoseErrString()
}

type ErrNodeAddressNotFound struct {
	NodeName string
	vconError.BaseError
}

// *********************************************** ErrNoDefaultSecgroup ************************************************

func NewErrNoDefaultSecgroup(pProjectUUID, pInfo string) vconError.IErrorBuilder {
	err := new(ErrNoDefaultSecgroup)
	err.ProjectUUID = pProjectUUID
	if pInfo != "" {
		err.Info = pInfo
	}
	return err
}

func IsErrNoDefaultSecgroup(pErr error) bool {
	_, ok := pErr.(*ErrNoDefaultSecgroup)
	return ok
}

func (s *ErrNoDefaultSecgroup) Error() string {
	s.DefaultError = fmt.Sprintf("the project %s has no default secgroup", s.ProjectUUID)
	return s.ChoseErrString()
}

type ErrNoDefaultSecgroup struct {
	ProjectUUID string
	vconError.BaseError
}

// ************************************************ ErrConflictService *************************************************

func NewErrConflictService(pPort int, pProtocol, pInfo string) vconError.IErrorBuilder {
	err := new(ErrConflictService)
	err.Port = pPort
	err.Protocol = pProtocol
	if pInfo != "" {
		err.Info = pInfo
	}
	return err
}

func IsErrConflictService(pErr error) bool {
	_, ok := pErr.(*ErrNoDefaultSecgroup)
	return ok
}

func (s *ErrConflictService) Error() string {
	s.DefaultError = fmt.Sprintf("the port [%d] or protocol [%s] MAY be conflicted with other service", s.Port, s.Protocol)
	return s.ChoseErrString()
}

type ErrConflictService struct {
	Port     int
	Protocol string
	vconError.BaseError
}

// ************************************************ ErrServicePortEmpty ************************************************

func NewErrServicePortEmpty() vconError.IErrorBuilder {
	err := new(ErrServicePortEmpty)
	return err
}

func IsErrServicePortEmpty(pErr error) bool {
	_, ok := pErr.(*ErrServicePortEmpty)
	return ok
}

func (s *ErrServicePortEmpty) Error() string {
	s.DefaultError = "service port is empty"
	return s.ChoseErrString()
}

type ErrServicePortEmpty struct {
	vconError.BaseError
}

// ************************************************** NoNodeAvailable **************************************************

func NewNoNodeAvailable() vconError.IErrorBuilder {
	err := new(NoNodeAvailable)
	return err
}

func IsNoNodeAvailable(pErr error) bool {
	_, ok := pErr.(*NoNodeAvailable)
	return ok
}

func (s *NoNodeAvailable) Error() string {
	s.DefaultError = "no node available in the cluster"
	return s.ChoseErrString()
}

type NoNodeAvailable struct {
	vconError.BaseError
}

// ********************************************* ErrConflictServiceAndCloud ********************************************

func NewErrConflictServiceAndCloud(pInfo string) vconError.IErrorBuilder {
	err := new(ErrConflictServiceAndCloud)
	err.Info = pInfo
	return err
}

func IsErrConflictServiceAndCloud(pErr error) bool {
	_, ok := pErr.(*ErrConflictServiceAndCloud)
	return ok
}

func (s *ErrConflictServiceAndCloud) Error() string {
	s.DefaultError = "conflict between service and VNG CLOUD"
	return s.ChoseErrString()
}

type ErrConflictServiceAndCloud struct {
	vconError.BaseError
}

var (
	ErrNotFound                           = vErr.New("failed to find object")
	ErrNoNodeAvailable                    = vErr.New("no node available in the cluster")
	ErrNoPortIsConfigured                 = vErr.New("no port is configured for the Kubernetes service")
	ErrSpecAndExistingLbNotMatch          = vErr.New("the spec and the existing load balancer do not match")
	ErrNodesAreNotSameSubnet              = vErr.New("nodes are not in the same subnet")
	ErrServiceConfigIsNil                 = vErr.New("service config is nil")
	ErrEmptyLoadBalancerAnnotation        = vErr.New("load balancer annotation is empty")
	ErrTimeoutWaitingLoadBalancerReady    = vErr.New("timeout waiting for load balancer to be ready")
	ErrLoadBalancerIdNotInAnnotation      = vErr.New("loadbalancer ID has been not set in service annotation")
	ErrServerObjectIsNil                  = vErr.New("server object is nil")
	ErrVngCloudNotReturnNilPoolObject     = vErr.New("VNG CLOUD should not return nil pool object")
	ErrVngCloudNotReturnNilListenerObject = vErr.New("VNG CLOUD should not return nil listener object")
	ErrIngressNotFound                    = vErr.New("failed to find ingress")
	ErrSecurityGroupNotFound              = vErr.New("failed to find security group")
)

var ErrLoadBalancerIDNotFoundAnnotation = vErr.New("failed to find LoadBalancerID from Annotation")
var ErrLoadBalancerNameNotFoundAnnotation = vErr.New("failed to find LoadBalancerName from Annotation")
