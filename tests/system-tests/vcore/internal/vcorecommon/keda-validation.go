package vcorecommon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/openshift-kni/eco-goinfra/pkg/configmap"
	"github.com/openshift-kni/eco-goinfra/pkg/deployment"
	"github.com/openshift-kni/eco-goinfra/pkg/monitoring"
	"github.com/openshift-kni/eco-goinfra/pkg/rbac"
	"github.com/openshift-kni/eco-goinfra/pkg/secret"
	"github.com/openshift-kni/eco-goinfra/pkg/service"
	"github.com/openshift-kni/eco-goinfra/pkg/serviceaccount"
	"github.com/openshift-kni/eco-gotests/tests/system-tests/internal/await"
	monv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/openshift-kni/eco-goinfra/pkg/reportxml"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/golang/glog"
	kedav1alpha1 "github.com/kedacore/keda-olm-operator/apis/keda/v1alpha1"
	kedav2v1alpha1 "github.com/kedacore/keda/v2/apis/keda/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openshift-kni/eco-goinfra/pkg/keda"
	"github.com/openshift-kni/eco-goinfra/pkg/namespace"
	"github.com/openshift-kni/eco-gotests/tests/system-tests/internal/apiobjectshelper"
	"github.com/openshift-kni/eco-gotests/tests/system-tests/internal/mirroring"
	"github.com/openshift-kni/eco-gotests/tests/system-tests/internal/ocpcli"
	"github.com/openshift-kni/eco-gotests/tests/system-tests/internal/platform"

	. "github.com/openshift-kni/eco-gotests/tests/system-tests/vcore/internal/vcoreinittools"
	"github.com/openshift-kni/eco-gotests/tests/system-tests/vcore/internal/vcoreparams"
)

const (
	kedaScaledObjectName      = "prometheus-scaledobject"
	configmapName             = "cluster-monitoring-config"
	configmapNamespace        = "openshift-monitoring"
	testAppServiceMonitorName = "keda-testing-sm"
	serviceAccountName        = "thanos"
	saTokenName               = "thanos-secret"
	triggerAuthName           = "keda-trigger-auth-prometheus"
	metricsReaderName         = "thanos-metrics-reader"

	prometheusOriginMirrorURL = "quay.io/zroubalik"
	prometheusImageName       = "prometheus-app"
	prometheusImageTag        = "latest"

	abOriginMirrorURL = "docker.io/jordi"
	abImageName       = "ab"
	abImageTag        = "latest"
)

// VerifyKedaSuite container that contains tests for Keda verification.
func VerifyKedaSuite() {
	Describe(
		"Keda validation",
		Label(vcoreparams.LabelVCoreOperators), func() {
			BeforeAll(func() {
				By(fmt.Sprintf("Asserting %s folder exists", vcoreparams.ConfigurationFolderName))

				homeDir, err := os.UserHomeDir()
				Expect(err).To(BeNil(), fmt.Sprint(err))

				vcoreConfigsFolder := filepath.Join(homeDir, vcoreparams.ConfigurationFolderName)

				glog.V(vcoreparams.VCoreLogLevel).Infof("vcoreConfigsFolder: %s", vcoreConfigsFolder)

				if err := os.Mkdir(vcoreConfigsFolder, 0755); os.IsExist(err) {
					glog.V(vcoreparams.VCoreLogLevel).Infof("%s folder already exists", vcoreConfigsFolder)
				}
			})

			It(fmt.Sprintf("Verifies %s namespace exists", vcoreparams.KedaNamespace),
				Label("keda"), VerifyKedaNamespaceExists)

			It("Verifies Keda operator deployment succeeded",
				Label("keda"), reportxml.ID("65001"), VerifyKedaDeployment)

			It("Verifies KedaController instance created successfully",
				Label("keda"), reportxml.ID("65004"), VerifyKedaControllerDeployment)

			It("Verifies ScaleObject instance created successfully",
				Label("keda"), reportxml.ID("65007"), VerifyScaleObjectDeployment)

			AfterAll(func() {
				By("Teardown")

				Expect(insureNamespaceNotExists(vcoreparams.KedaWatchNamespace)).
					To(Equal(true), fmt.Sprintf("Failed to delete watch namespace %s",
						vcoreparams.KedaWatchNamespace))
			})
		})
}

// VerifyKedaNamespaceExists asserts namespace for NMState operator exists.
func VerifyKedaNamespaceExists(ctx SpecContext) {
	err := apiobjectshelper.VerifyNamespaceExists(APIClient, vcoreparams.KedaNamespace, time.Second)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to pull %q namespace", vcoreparams.KedaNamespace))
} // func VerifyKedaNamespaceExists (ctx SpecContext)

// VerifyKedaDeployment assert that Keda operator deployment succeeded.
func VerifyKedaDeployment(ctx SpecContext) {
	err := apiobjectshelper.VerifyOperatorDeployment(APIClient,
		vcoreparams.KedaSubscriptionName,
		vcoreparams.KedaDeploymentName,
		vcoreparams.KedaNamespace,
		time.Minute)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Keda operator deployment %s failure in the namespace %s; %v",
			vcoreparams.KedaDeploymentName, vcoreparams.KedaNamespace, err))
} // func VerifyKedaDeployment (ctx SpecContext)

// VerifyKedaControllerDeployment assert that kedaController instance created successfully.
func VerifyKedaControllerDeployment(ctx SpecContext) {
	glog.V(vcoreparams.VCoreLogLevel).Infof("Verify kedaController instance exists")

	kedaControllerBuilder := keda.NewControllerBuilder(
		APIClient,
		vcoreparams.KedaControllerName,
		vcoreparams.KedaNamespace)

	if !kedaControllerBuilder.Exists() {
		var err error

		admissionWebhooks := kedav1alpha1.KedaAdmissionWebhooksSpec{
			LogLevel:   "info",
			LogEncoder: "console",
		}
		operator := kedav1alpha1.KedaOperatorSpec{
			LogLevel:   "info",
			LogEncoder: "console",
		}
		metricsServer := kedav1alpha1.KedaMetricsServerSpec{
			LogLevel: "0",
		}

		kedaControllerBuilder, err = kedaControllerBuilder.WithAdmissionWebhooks(admissionWebhooks).
			WithOperator(operator).
			WithMetricsServer(metricsServer).
			WithWatchNamespace(vcoreparams.KedaWatchNamespace).
			Create()
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Failed to create kedaController instance %q in namespace %q due to: %v",
				vcoreparams.KedaControllerName, vcoreparams.KedaNamespace, err))
		Expect(kedaControllerBuilder.Exists()).To(Equal(true), fmt.Sprintf(
			"no kedaController instance  %s was found in the namespace %s",
			vcoreparams.KedaControllerName, vcoreparams.KedaNamespace))
	}
} // func VerifyKedaControllerDeployment (ctx SpecContext)

// VerifyScaleObjectDeployment assert that scaleObject instance created successfully.
//
//nolint:funlen
func VerifyScaleObjectDeployment(ctx SpecContext) {
	glog.V(vcoreparams.VCoreLogLevel).Info("Verify monitoring status")

	var err error

	configMapBuilder := configmap.NewBuilder(APIClient, configmapName, configmapNamespace)

	if !configMapBuilder.Exists() {
		configMapBuilder, err = configMapBuilder.
			WithData(map[string]string{"config.yaml": "enableUserWorkload: true"}).Create()
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Failed to create configmap %q in namespace %q due to: %v",
				configmapName, configmapNamespace, err))
		Expect(configMapBuilder.Exists()).To(Equal(true), fmt.Sprintf(
			"no configmap %s was found in the namespace %s", configmapName, configmapNamespace))
	}

	glog.V(vcoreparams.VCoreLogLevel).Info("Deploy application that exposes Prometheus metrics")

	Expect(insureNamespaceNotExists(vcoreparams.KedaWatchNamespace)).
		To(Equal(true), fmt.Sprintf("Failed to delete watch namespace %s",
			vcoreparams.KedaWatchNamespace))

	Expect(insureNamespaceExists(vcoreparams.KedaWatchNamespace)).To(Equal(true),
		fmt.Sprintf("failed to create namespace %s", vcoreparams.KedaWatchNamespace))

	glog.V(vcoreparams.VCoreLogLevel).Infof("Create test application deployment %s in namespace %s",
		vcoreparams.KedaWatchAppName, vcoreparams.KedaWatchNamespace)

	prometeusImageURL, err := getImageURL(prometheusOriginMirrorURL, prometheusImageName, prometheusImageTag)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to generate prometheus image URL for %s/%s:%s due to: %v",
			prometheusOriginMirrorURL, prometheusImageName, prometheusImageTag, err))

	falseVar := false
	trueVar := true
	securityContext := corev1.SecurityContext{
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
		RunAsNonRoot:             &trueVar,
		AllowPrivilegeEscalation: &falseVar,
		SeccompProfile: &corev1.SeccompProfile{
			Type: "RuntimeDefault",
		},
	}

	appConteiner := corev1.Container{
		Name:            "prom-test-app",
		Image:           prometeusImageURL,
		ImagePullPolicy: "IfNotPresent",
		SecurityContext: &securityContext,
	}

	_, err = deployment.NewBuilder(APIClient,
		vcoreparams.KedaWatchAppName,
		vcoreparams.KedaWatchNamespace,
		map[string]string{"app": vcoreparams.KedaWatchAppName},
		&appConteiner,
	).WithLabel("type", "keda-testing").WithReplicas(int32(1)).Create()
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create test application %s in namespace %s due to: %v",
			vcoreparams.KedaWatchAppName, vcoreparams.KedaWatchNamespace, err))

	glog.V(vcoreparams.VCoreLogLevel).Infof("Create test application Service %s in namespace %s",
		vcoreparams.KedaWatchAppName, vcoreparams.KedaWatchNamespace)

	_, err = service.NewBuilder(APIClient,
		vcoreparams.KedaWatchAppName,
		vcoreparams.KedaWatchNamespace,
		map[string]string{"type": "keda-testing"},
		corev1.ServicePort{
			Name:     "http",
			Protocol: "TCP",
			Port:     80,
			TargetPort: intstr.IntOrString{
				Type:   intstr.Int,
				IntVal: int32(8080),
			},
		},
	).WithAnnotation(map[string]string{"prometheus.io/scrape": "true"}).
		WithLabels(map[string]string{"app": vcoreparams.KedaWatchAppName}).Create()
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create service %s in namespace %s due to: %v",
			vcoreparams.KedaWatchAppName, vcoreparams.KedaWatchNamespace, err))

	glog.V(vcoreparams.VCoreLogLevel).Infof("Create ServiceMonitor %s in namespace %s",
		testAppServiceMonitorName, vcoreparams.KedaWatchNamespace)

	endpoints := []monv1.Endpoint{{
		Port:   "http",
		Scheme: "http",
	}}

	_, err = monitoring.NewBuilder(APIClient,
		testAppServiceMonitorName,
		vcoreparams.KedaWatchNamespace).WithEndpoints(endpoints).
		WithSelector(map[string]string{"app": vcoreparams.KedaWatchAppName}).Create()
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create serviceMonitor %s in namespace %s due to: %v",
			testAppServiceMonitorName, vcoreparams.KedaWatchNamespace, err))

	glog.V(vcoreparams.VCoreLogLevel).Infof("Create test application ServiceAccount %s in namespace %s "+
		"and locate assigned token",
		vcoreparams.KedaWatchAppName, vcoreparams.KedaWatchNamespace)

	_, err = serviceaccount.NewBuilder(APIClient,
		serviceAccountName, vcoreparams.KedaWatchNamespace).Create()
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create serviceAccount %s in namespace %s due to: %v",
			serviceAccountName, vcoreparams.KedaWatchNamespace, err))

	_, err = secret.NewBuilder(APIClient,
		saTokenName,
		vcoreparams.KedaWatchNamespace,
		corev1.SecretTypeServiceAccountToken).
		WithAnnotations(map[string]string{"kubernetes.io/service-account.name": serviceAccountName}).Create()
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create token assigned to the serviceAccount %s in namespace %s due to: %v",
			serviceAccountName, vcoreparams.KedaWatchNamespace, err))

	glog.V(vcoreparams.VCoreLogLevel).
		Infof("Define TriggerAuthentication %s with the Service Account's token %s in namespace %s",
			triggerAuthName, vcoreparams.KedaWatchNamespace, saTokenName)

	secretTargetRef := []kedav2v1alpha1.AuthSecretTargetRef{{
		Parameter: "bearerToken",
		Name:      saTokenName,
		Key:       "token",
	}, {
		Parameter: "ca",
		Name:      saTokenName,
		Key:       "ca.crt",
	}}

	_, err = keda.NewTriggerAuthenticationBuilder(APIClient,
		triggerAuthName,
		vcoreparams.KedaWatchNamespace).WithSecretTargetRef(secretTargetRef).Create()
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create TriggerAuthentication %s with the Service Account's token %s "+
			"in namespace %s due to: %v",
			triggerAuthName, vcoreparams.KedaWatchNamespace, saTokenName, err))

	glog.V(vcoreparams.VCoreLogLevel).Infof("Create a role %s for reading metric from Thanos in namespace %s",
		metricsReaderName, vcoreparams.KedaWatchNamespace)

	roleRule1 := rbacv1.PolicyRule{
		APIGroups: []string{""},
		Resources: []string{"pods"},
		Verbs:     []string{"get"},
	}
	roleRule2 := rbacv1.PolicyRule{
		APIGroups: []string{"metrics.k8s.io"},
		Resources: []string{"pods", "nodes"},
		Verbs:     []string{"get", "list", "watch"},
	}

	_, err = rbac.NewRoleBuilder(APIClient, metricsReaderName, vcoreparams.KedaWatchNamespace, roleRule1).
		WithRules([]rbacv1.PolicyRule{roleRule2}).Create()
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create a role %s for reading metric from Thanos in namespace %s due to: %v",
			metricsReaderName, vcoreparams.KedaWatchNamespace, err))

	glog.V(vcoreparams.VCoreLogLevel).Infof("Create a roleBinding %s for serviceaccount %s in namespace %s",
		metricsReaderName, serviceAccountName, vcoreparams.KedaWatchNamespace)

	kedaRoleBindingTemplateName := "keda-rolebinding.yaml"
	varsToReplace := make(map[string]interface{})
	varsToReplace["RoleBindingName"] = metricsReaderName
	varsToReplace["RoleBindingNamespace"] = vcoreparams.KedaWatchNamespace
	varsToReplace["ServiceAccountName"] = serviceAccountName
	varsToReplace["RoleName"] = metricsReaderName
	homeDir, err := os.UserHomeDir()
	Expect(err).ToNot(HaveOccurred(), "user home directory not found; %s", err)

	destinationDirectoryPath := filepath.Join(homeDir, vcoreparams.ConfigurationFolderName)

	workingDir, err := os.Getwd()
	Expect(err).ToNot(HaveOccurred(), err)

	templateDir := filepath.Join(workingDir, vcoreparams.TemplateFilesFolder)

	err = ocpcli.CreateConfig(
		templateDir,
		kedaRoleBindingTemplateName,
		destinationDirectoryPath,
		kedaRoleBindingTemplateName,
		varsToReplace)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to create load job due to %v", err))

	glog.V(vcoreparams.VCoreLogLevel).Infof("Define scaledObject instance %s in namespace %s",
		kedaScaledObjectName, vcoreparams.KedaWatchNamespace)

	scaledObjectBuilder := keda.NewScaledObjectBuilder(APIClient, kedaScaledObjectName, vcoreparams.KedaWatchNamespace)

	scaleTargetRef := kedav2v1alpha1.ScaleTarget{
		Name: vcoreparams.KedaWatchAppName,
	}

	scaleTriggers := []kedav2v1alpha1.ScaleTriggers{{
		Type: "prometheus",
		Metadata: map[string]string{
			"serverAddress": "https://thanos-querier.openshift-monitoring.svc.cluster.local:9092",
			"namespace":     vcoreparams.KedaWatchNamespace,
			"metricName":    "http_requests_total",
			"threshold":     "5",
			"query":         "sum(rate(http_requests_total{job=\"test-app\"}[1m]))",
			"authModes":     "bearer",
		},
		AuthenticationRef: &kedav2v1alpha1.AuthenticationRef{
			Name: triggerAuthName,
			Kind: "TriggerAuthentication",
		},
	}}

	_, err = scaledObjectBuilder.WithScaleTargetRef(scaleTargetRef).
		WithMinReplicaCount(int32(1)).
		WithMaxReplicaCount(int32(8)).
		WithPollingInterval(int32(5)).
		WithCooldownPeriod(int32(10)).
		WithTriggers(scaleTriggers).Create()
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create scaledObject instance %s in namespace %s due to: %v",
			kedaScaledObjectName, vcoreparams.KedaWatchNamespace, err))

	glog.V(vcoreparams.VCoreLogLevel).Info("Generate requests to test the application autoscaling")

	abImageURL, err := getImageURL(abOriginMirrorURL, abImageName, abImageTag)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to generate prometheus image URL for %s/%s:%s due to: %v",
			prometheusOriginMirrorURL, prometheusImageName, prometheusImageTag, err))

	appLoadJobTemplateName := "keda-test-app-load-job.yaml"
	varsToReplace["KedaWatchNamespace"] = vcoreparams.KedaWatchNamespace
	varsToReplace["TestNamespace"] = vcoreparams.KedaWatchNamespace
	varsToReplace["AbImageURL"] = abImageURL

	err = ocpcli.CreateConfig(
		templateDir,
		appLoadJobTemplateName,
		destinationDirectoryPath,
		appLoadJobTemplateName,
		varsToReplace)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to create load job due to %v", err))

	glog.V(vcoreparams.VCoreLogLevel).Info("Wait until pods replicas count reach 8")

	isCntReached, err := await.WaitForThePodReplicasCountInNamespace(APIClient,
		vcoreparams.KedaWatchNamespace, metav1.ListOptions{
			LabelSelector: "app=test-app",
		}, 8, time.Minute*5)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to scale %s pods in namespace %s due to %v",
		vcoreparams.KedaWatchAppName, vcoreparams.KedaWatchNamespace, err))
	Expect(isCntReached).To(Equal(true), fmt.Sprintf("failed to scale %s pods in namespace %s after %v",
		vcoreparams.KedaWatchAppName, vcoreparams.KedaWatchNamespace, time.Minute*5))
} // func VerifyKedaControllerDeployment (ctx SpecContext)

func getImageURL(repository, name, tag string) (string, error) {
	imageURL := fmt.Sprintf("%s/%s", repository, name)

	isDisconnected, err := platform.IsDisconnectedDeployment(APIClient)

	if err != nil {
		return "", err
	}

	if !isDisconnected {
		glog.V(vcoreparams.VCoreLogLevel).Info("The connected deployment type was detected, " +
			"the images mirroring is not required")
	} else {
		glog.V(vcoreparams.VCoreLogLevel).Infof("Mirror image %s:%s locally", imageURL, tag)

		imageURL, _, err = mirroring.MirrorImageToTheLocalRegistry(
			APIClient,
			repository,
			name,
			tag,
			VCoreConfig.Host,
			VCoreConfig.User,
			VCoreConfig.Pass,
			VCoreConfig.CombinedPullSecretFile,
			VCoreConfig.RegistryRepository)

		if err != nil {
			return "", fmt.Errorf("failed to mirror image %s:%s locally due to %w",
				name, tag, err)
		}
	}

	return fmt.Sprintf("%s:%s", imageURL, tag), nil
}

func insureNamespaceNotExists(nsName string) bool {
	watchNamespace := namespace.NewBuilder(APIClient, nsName)
	if watchNamespace.Exists() {
		err := watchNamespace.Delete()
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Failed to delete watch namespace %s due to: %v",
				vcoreparams.KedaWatchNamespace, err))

		err = wait.PollUntilContextTimeout(
			context.TODO(),
			time.Second,
			time.Minute*10,
			true,
			func(ctx context.Context) (bool, error) {
				isExists := watchNamespace.Exists()

				if !isExists {
					return true, nil
				}

				return false, nil
			})
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Failed to delete watch namespace %s due to: %v",
				vcoreparams.KedaWatchNamespace, err))
	}

	return true
}

func insureNamespaceExists(nsName string) bool {
	glog.V(vcoreparams.VCoreLogLevel).Infof("Insure namespace %q exists", nsName)

	createNs := namespace.NewBuilder(APIClient, nsName)

	if !createNs.Exists() {
		createNs, err := createNs.Create()

		if err != nil {
			glog.V(vcoreparams.VCoreLogLevel).Infof("Error creating namespace %q: %v", nsName, err)

			return false
		}

		err = wait.PollUntilContextTimeout(
			context.TODO(),
			time.Second,
			3*time.Second,
			true,
			func(ctx context.Context) (bool, error) {
				if !createNs.Exists() {
					glog.V(vcoreparams.VCoreLogLevel).Infof("Error creating namespace %q", nsName)

					return false, nil
				}

				glog.V(vcoreparams.VCoreLogLevel).Infof("Created namespace %q", createNs.Definition.Name)

				return true, nil
			})

		if err != nil {
			return false
		}
	}

	return true
}
