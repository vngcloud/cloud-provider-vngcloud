package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/vngcloud/cloud-provider-vngcloud/cmd/vngcloud-ic-webhook/admission"
	"github.com/vngcloud/cloud-provider-vngcloud/pkg/utils"
	admissionv1 "k8s.io/api/admission/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	TLS_PATH      = "/tmp/vngcloud-ic-webhook"
	MUTATE_PATH   = "/mutate"
	VALIDATE_PATH = "/validate"
	HEALTH_PATH   = "/health"
	PORT_HTTPS    = 8443
	PORT_HTTP     = 8080
)

func PointerOf[T any](t T) *T {
	return &t
}

var (
	commonName string
	Haha       string

	isCreateSecret       bool
	isCreateWehookConfig bool
	namespace            string

	debug bool
)

func init() {
	flag.StringVar(&commonName, "common-name", "", "common name for the server certificate")

	flag.BoolVar(&isCreateSecret, "create-secret", true, "create secret")
	flag.BoolVar(&isCreateWehookConfig, "create-webhook-config", true, "create webhook config")
	flag.StringVar(&namespace, "namespace", "default", "namespace as defined by .metadata.namespace")

	flag.BoolVar(&debug, "debug", true, "enable debug")

	flag.Parse()

	// Generate CA certificate and key
	caCert, caKey, err := generateCACertificate()
	if err != nil {
		log.Fatalf("Error generating CA certificate: %s", err)
	}

	// Generate server certificate and key
	serverCert, serverKey, err := generateServerCertificate(caCert, caKey, commonName+"."+namespace+".svc")
	if err != nil {
		log.Fatalf("Error generating server certificate: %s", err)
	}

	if isCreateSecret {
		// Build the Kubernetes client
		clientset := buildClientset()

		// Check if exist then delete
		_, err := clientset.CoreV1().Secrets(namespace).Get(context.TODO(), commonName, metav1.GetOptions{})
		if err != nil {
			log.Printf("Secret %s not found in namespace %s", commonName, namespace)
		} else {
			err = clientset.CoreV1().Secrets(namespace).Delete(context.TODO(), commonName, metav1.DeleteOptions{})
			if err != nil {
				log.Fatalf("Error deleting secret: %s", err)
			}
		}

		// Create Kubernetes secrets
		err = createSecret(clientset, namespace, commonName, map[string][]byte{
			"tls.crt": pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverCert.Raw}),
			"tls.key": pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(serverKey)}),
			"ca.crt":  pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCert.Raw}),
		})
		if err != nil {
			log.Fatalf("Error creating server TLS secret: %s", err)
		}
		log.Printf("Secret %s/%s created successfully\n", namespace, commonName)
	}

	log.Println("Certificates and secrets generated successfully")

	// Check if the folder exists
	if _, err := os.Stat(TLS_PATH); os.IsNotExist(err) {
		err := os.MkdirAll(TLS_PATH, os.ModePerm)
		if err != nil {
			fmt.Println("Error creating folder:", err)
			return
		}
		fmt.Println("Folder created successfully.")
	} else {
		fmt.Println("Folder already exists.")
	}
	writeToFile(TLS_PATH+"/ca.crt", "CERTIFICATE", caCert.Raw)
	writeToFile(TLS_PATH+"/ca.key", "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(caKey))
	writeToFile(TLS_PATH+"/tls.crt", "CERTIFICATE", serverCert.Raw)
	writeToFile(TLS_PATH+"/tls.key", "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(serverKey))

	if isCreateWehookConfig {
		clientset := buildClientset()

		// Check if exist then delete
		_, err = clientset.AdmissionregistrationV1().ValidatingWebhookConfigurations().Get(context.TODO(), commonName, metav1.GetOptions{})
		if err != nil {
			log.Printf("ValidatingWebhookConfiguration: %s not found", commonName)
		} else {
			err = clientset.AdmissionregistrationV1().ValidatingWebhookConfigurations().Delete(context.TODO(), commonName, metav1.DeleteOptions{})
			if err != nil {
				log.Fatalf("Error deleting ValidatingWebhookConfiguration: %s", err)
			}
		}

		validateconfig := &admissionregistrationv1.ValidatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name: commonName,
			},
			Webhooks: []admissionregistrationv1.ValidatingWebhook{{
				Name: commonName + ".vngcloud.vn",
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					CABundle: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCert.Raw}),
					Service: &admissionregistrationv1.ServiceReference{
						Name:      commonName,
						Namespace: namespace,
						Path:      PointerOf(VALIDATE_PATH),
						Port:      PointerOf(int32(PORT_HTTPS)),
					},
				},
				Rules: []admissionregistrationv1.RuleWithOperations{{
					Operations: []admissionregistrationv1.OperationType{admissionregistrationv1.Create, admissionregistrationv1.Update},
					Rule: admissionregistrationv1.Rule{
						APIGroups:   []string{"networking.k8s.io"},
						APIVersions: []string{"v1"},
						Resources:   []string{"ingresses"},
					},
				}},
				FailurePolicy:           PointerOf(admissionregistrationv1.Fail),
				SideEffects:             PointerOf(admissionregistrationv1.SideEffectClassNone),
				AdmissionReviewVersions: []string{"v1"},
			}},
		}
		if _, err := clientset.AdmissionregistrationV1().ValidatingWebhookConfigurations().Create(context.TODO(), validateconfig, metav1.CreateOptions{}); err != nil {
			log.Fatalf("Error creating ValidatingWebhookConfiguration: %s", err)
		}
		log.Printf("ValidatingWebhookConfiguration %s created successfully\n", commonName)
	}
}

func generateCACertificate() (*x509.Certificate, *rsa.PrivateKey, error) {
	// Generate private key
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	// Create CA certificate
	caCertTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2024),
		Subject: pkix.Name{
			Organization: []string{},
			CommonName:   commonName,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	caCertBytes, err := x509.CreateCertificate(rand.Reader, caCertTemplate, caCertTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, err
	}

	caCert, err := x509.ParseCertificate(caCertBytes)
	if err != nil {
		return nil, nil, err
	}

	return caCert, caKey, nil
}

func generateServerCertificate(caCert *x509.Certificate, caKey *rsa.PrivateKey, commonName string) (*x509.Certificate, *rsa.PrivateKey, error) {
	// Generate private key
	serverKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	// Create server certificate
	serverCertTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2025),
		Subject: pkix.Name{
			Organization: []string{},
			CommonName:   commonName,
		},
		DNSNames:    []string{commonName},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(1 * 365 * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	serverCertBytes, err := x509.CreateCertificate(rand.Reader, serverCertTemplate, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, err
	}

	serverCert, err := x509.ParseCertificate(serverCertBytes)
	if err != nil {
		return nil, nil, err
	}

	return serverCert, serverKey, nil
}

func createSecret(clientset *kubernetes.Clientset, namespace string, name string, data map[string][]byte) error {
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: data,
	}

	_, err := clientset.CoreV1().Secrets(namespace).Create(context.TODO(), secret, metav1.CreateOptions{})
	return err
}

func writeToFile(filename, blockType string, data []byte) {
	file, err := os.Create(filename)
	if err != nil {
		log.Fatalf("Error creating file %s: %s", filename, err)
	}
	defer file.Close()

	err = pem.Encode(file, &pem.Block{Type: blockType, Bytes: data})
	if err != nil {
		log.Fatalf("Error writing to file %s: %s", filename, err)
	}
	log.Printf("Written %s\n", filename)
}

// Build the Kubernetes client should mount config in /etc/kubernetes/...
func buildClientset() *kubernetes.Clientset {
	// initialize k8s client
	clientset, err := utils.CreateApiserverClient("", "")
	if err != nil {
		log.Fatalf("Error building kubernetes clientset: %s", err)
	}
	return clientset
}

func main() {
	setLogger()

	// handle our core application
	http.HandleFunc(VALIDATE_PATH, ServeValidate)
	http.HandleFunc(HEALTH_PATH, ServeHealth)

	// start the server
	// listens to clear text http on port ... unless TLS env var is set to "true"
	if os.Getenv("TLS") == "true" {
		cert := TLS_PATH + "/tls.crt"
		key := TLS_PATH + "/tls.key"
		logrus.Printf("Listening on port %d ...", PORT_HTTPS)
		logrus.Fatal(http.ListenAndServeTLS(fmt.Sprintf(":%d", PORT_HTTPS), cert, key, nil))
	} else {
		logrus.Printf("Listening on port %d ...", PORT_HTTP)
		logrus.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", PORT_HTTP), nil))
	}
}

// setLogger sets the logger using env vars, it defaults to text logs on
// debug level unless otherwise specified
func setLogger() {
	logrus.SetLevel(logrus.DebugLevel)

	lev := os.Getenv("LOG_LEVEL")
	if lev != "" {
		llev, err := logrus.ParseLevel(lev)
		if err != nil {
			logrus.Fatalf("cannot set LOG_LEVEL to %q", lev)
		}
		logrus.SetLevel(llev)
	}

	if os.Getenv("LOG_JSON") == "true" {
		logrus.SetFormatter(&logrus.JSONFormatter{})
	}
}

// ServeValidate validates an admission request and then writes an admission
// review to `w`
func ServeValidate(w http.ResponseWriter, r *http.Request) {
	logger := logrus.WithField("uri", r.RequestURI)
	logger.Debug("------------------------\nreceived validation request")

	in, err := parseRequest(*r)
	if err != nil {
		logger.Error(err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	adm := admission.Admitter{
		Logger:  logger,
		Request: in.Request,
	}

	out, err := adm.ValidateReview()
	if err != nil {
		e := fmt.Sprintf("could not generate admission response: %v", err)
		logger.Error(e)
		http.Error(w, e, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	jout, err := json.Marshal(out)
	if err != nil {
		e := fmt.Sprintf("could not parse admission response: %v", err)
		logger.Error(e)
		http.Error(w, e, http.StatusInternalServerError)
		return
	}

	logger.Debug("sending response")
	logger.Debugf("%s", jout)
	fmt.Fprintf(w, "%s", jout)
}

// ServeHealth returns 200 when things are good
func ServeHealth(w http.ResponseWriter, r *http.Request) {
	logrus.WithField("uri", r.RequestURI).Debug("healthy")
	fmt.Fprint(w, "OK")
}

// parseRequest extracts an AdmissionReview from an http.Request if possible
func parseRequest(r http.Request) (*admissionv1.AdmissionReview, error) {
	if r.Header.Get("Content-Type") != "application/json" {
		return nil, fmt.Errorf("Content-Type: %q should be %q",
			r.Header.Get("Content-Type"), "application/json")
	}

	bodybuf := new(bytes.Buffer)
	bodybuf.ReadFrom(r.Body)
	body := bodybuf.Bytes()

	if len(body) == 0 {
		return nil, fmt.Errorf("admission request body is empty")
	}

	var a admissionv1.AdmissionReview

	if err := json.Unmarshal(body, &a); err != nil {
		return nil, fmt.Errorf("could not parse admission review request: %v", err)
	}

	if a.Request == nil {
		return nil, fmt.Errorf("admission review can't be used: Request field is nil")
	}

	return &a, nil
}
