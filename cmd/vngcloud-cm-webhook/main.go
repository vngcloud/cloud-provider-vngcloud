package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"log"
	"math/big"
	"os"
	"time"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func PointerOf[T any](t T) *T {
	return &t
}

var (
	commonName string
	Haha       string

	isCreateSecret       bool
	isCreateWehookConfig bool
	kubeconfig           string
	secretName           string
	namespace            string

	debug bool
)

func init() {
	flag.StringVar(&commonName, "common-name", "webhook-service.default.svc", "common name for the server certificate")

	flag.BoolVar(&isCreateSecret, "create-secret", false, "create secret")
	flag.BoolVar(&isCreateWehookConfig, "create-webhook-config", true, "create webhook config")
	flag.StringVar(&kubeconfig, "kubeconfig", "", "absolute path to the kubeconfig file")
	flag.StringVar(&secretName, "secret-name", "webhook-server-tls", "secret name for the server certificate")
	flag.StringVar(&namespace, "namespace", "default", "namespace as defined by pod.metadata.namespace")

	flag.BoolVar(&debug, "debug", true, "enable debug")

	flag.Parse()

	// Generate CA certificate and key
	caCert, caKey, err := generateCACertificate()
	if err != nil {
		log.Fatalf("Error generating CA certificate: %s", err)
	}

	// Generate server certificate and key
	serverCert, serverKey, err := generateServerCertificate(caCert, caKey, commonName)
	if err != nil {
		log.Fatalf("Error generating server certificate: %s", err)
	}

	if isCreateSecret {
		// Build the Kubernetes client
		clientset := buildClientset(kubeconfig)

		// Check if exist then delete
		_, err := clientset.CoreV1().Secrets(namespace).Get(context.TODO(), secretName, metav1.GetOptions{})
		if err != nil {
			log.Printf("Secret %s not found in namespace %s", secretName, namespace)
		} else {
			err = clientset.CoreV1().Secrets(namespace).Delete(context.TODO(), secretName, metav1.DeleteOptions{})
			if err != nil {
				log.Fatalf("Error deleting secret: %s", err)
			}
		}

		// Create Kubernetes secrets
		err = createSecret(clientset, namespace, secretName, map[string][]byte{
			"tls.crt": pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverCert.Raw}),
			"tls.key": pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(serverKey)}),
			"ca.crt":  pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCert.Raw}),
		})
		if err != nil {
			log.Fatalf("Error creating server TLS secret: %s", err)
		}
		log.Printf("Secret %s/%s created successfully\n", namespace, secretName)
	}

	log.Println("Certificates and secrets generated successfully")

	if debug {
		// // print certificates
		// log.Println("CA Certificate:")
		// log.Println(string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCert.Raw})))
		// log.Println("Server Certificate:")
		// log.Println(string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverCert.Raw})))
		// log.Println("Server Private Key:")
		// log.Println(string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(serverKey)})))

		// write certificates to files
		writeToFile("ca.crt", "CERTIFICATE", caCert.Raw)
		writeToFile("ca.key", "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(caKey))
		writeToFile("server.crt", "CERTIFICATE", serverCert.Raw)
		writeToFile("server.key", "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(serverKey))
	}

	if isCreateWehookConfig {
		clientset := buildClientset(kubeconfig)

		// Check if exist then delete
		_, err := clientset.AdmissionregistrationV1().MutatingWebhookConfigurations().Get(context.TODO(), commonName, metav1.GetOptions{})
		if err != nil {
			log.Printf("MutatingWebhookConfiguration: %s not found", commonName)
		} else {
			err = clientset.AdmissionregistrationV1().MutatingWebhookConfigurations().Delete(context.TODO(), commonName, metav1.DeleteOptions{})
			if err != nil {
				log.Fatalf("Error deleting MutatingWebhookConfiguration %s: %s", commonName, err)
			}
		}

		_, err = clientset.AdmissionregistrationV1().ValidatingWebhookConfigurations().Get(context.TODO(), commonName, metav1.GetOptions{})
		if err != nil {
			log.Printf("ValidatingWebhookConfiguration: %s not found", commonName)
		} else {
			err = clientset.AdmissionregistrationV1().ValidatingWebhookConfigurations().Delete(context.TODO(), commonName, metav1.DeleteOptions{})
			if err != nil {
				log.Fatalf("Error deleting ValidatingWebhookConfiguration: %s", err)
			}
		}

		mutateconfig := &admissionregistrationv1.MutatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name: commonName,
			},
			Webhooks: []admissionregistrationv1.MutatingWebhook{{
				Name: commonName,
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					CABundle: caCert.Raw,
					Service: &admissionregistrationv1.ServiceReference{
						Name:      "vngcloud-controller-manager",
						Namespace: namespace,
						Path:      PointerOf("/mutate"),
					},
				},
				Rules: []admissionregistrationv1.RuleWithOperations{{
					Operations: []admissionregistrationv1.OperationType{admissionregistrationv1.Create, admissionregistrationv1.Update},
					Rule: admissionregistrationv1.Rule{
						APIGroups:   []string{"apps"},
						APIVersions: []string{"v1"},
						Resources:   []string{"deployments"},
					},
				}},
				FailurePolicy:           PointerOf(admissionregistrationv1.Fail),
				SideEffects:             PointerOf(admissionregistrationv1.SideEffectClassNone),
				AdmissionReviewVersions: []string{"v1"},
			}},
		}
		if _, err := clientset.AdmissionregistrationV1().MutatingWebhookConfigurations().Create(context.TODO(), mutateconfig, metav1.CreateOptions{}); err != nil {
			log.Fatalf("Error creating MutatingWebhookConfiguration: %s", err)
		}
		log.Printf("MutatingWebhookConfiguration %s created successfully\n", commonName)

		validateconfig := &admissionregistrationv1.ValidatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name: commonName,
			},
			Webhooks: []admissionregistrationv1.ValidatingWebhook{{
				Name: commonName,
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					CABundle: caCert.Raw,
					Service: &admissionregistrationv1.ServiceReference{
						Name:      "vngcloud-controller-manager",
						Namespace: namespace,
						Path:      PointerOf("/validate"),
					},
				},
				Rules: []admissionregistrationv1.RuleWithOperations{{
					Operations: []admissionregistrationv1.OperationType{admissionregistrationv1.Create, admissionregistrationv1.Update},
					Rule: admissionregistrationv1.Rule{
						APIGroups:   []string{"apps"},
						APIVersions: []string{"v1"},
						Resources:   []string{"deployments"},
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

// Build the Kubernetes client
func buildClientset(cfg string) *kubernetes.Clientset {
	config, err := clientcmd.BuildConfigFromFlags("", cfg)
	if err != nil {
		log.Fatalf("Error building kubeconfig: %s", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Error building kubernetes clientset: %s", err)
	}
	return clientset
}

func main() {

}
