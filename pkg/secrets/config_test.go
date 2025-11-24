package secrets

import (
	"context"
	"testing"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/cluster-inventory-api/apis/v1alpha1"
)

func TestSecrets(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Secrets Package Suite")
}

var _ = ginkgo.Describe("BuildConfigFromCP", func() {
	var (
		ctx           context.Context
		clientset     *fake.Clientset
		consumerName  string
		namespaceName string
		cpName        string
		cpNamespace   string
	)

	ginkgo.BeforeEach(func() {
		ctx = context.TODO()
		clientset = fake.NewSimpleClientset()
		consumerName = "my-consumer"
		namespaceName = "ci-consumer-ns"
		cpName = "workload-cluster"
		cpNamespace = "default"

		// Create consumer namespace with required label
		_, err := clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespaceName,
				Labels: map[string]string{
					LabelConsumerKey: consumerName,
				},
			},
		}, metav1.CreateOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// Create secret containing kubeconfig and labels for ClusterProfile
		kubeconfig := []byte(`apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://example.cluster.local
  name: c1
contexts:
- context:
    cluster: c1
    user: u1
  name: ctx1
current-context: ctx1
users:
- name: u1
  user:
    token: abcdefg
`)
		_, err = clientset.CoreV1().Secrets(namespaceName).Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster-credentials",
				Namespace: namespaceName,
				Labels: map[string]string{
					LabelClusterProfileName:      cpName,
					LabelClusterProfileNamespace: cpNamespace,
				},
			},
			Data: map[string][]byte{
				SecretDataKeyKubeconfig: kubeconfig,
			},
		}, metav1.CreateOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	})

	ginkgo.It("should discover secret by labels and build rest.Config successfully", func() {
		cp := &v1alpha1.ClusterProfile{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cpName,
				Namespace: cpNamespace,
			},
		}
		cfg, err := BuildConfigFromCP(ctx, clientset, consumerName, cp)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(cfg).NotTo(gomega.BeNil())
		gomega.Expect(cfg.Host).To(gomega.Equal("https://example.cluster.local"))
	})
})

var _ = ginkgo.Describe("BuildConfigFromCP - error handling", func() {
	ginkgo.It("should return error when kubeClient is nil", func() {
		cp := &v1alpha1.ClusterProfile{ObjectMeta: metav1.ObjectMeta{Name: "cp"}}
		cfg, err := BuildConfigFromCP(context.TODO(), nil, "consumer", cp)
		gomega.Expect(err).To(gomega.HaveOccurred())
		gomega.Expect(err.Error()).To(gomega.ContainSubstring("kubeClient must not be nil"))
		gomega.Expect(cfg).To(gomega.BeNil())
	})

	ginkgo.It("should return error when consumerName is empty", func() {
		clientset := fake.NewSimpleClientset()
		cp := &v1alpha1.ClusterProfile{ObjectMeta: metav1.ObjectMeta{Name: "cp"}}
		_, err := BuildConfigFromCP(context.TODO(), clientset, "", cp)
		gomega.Expect(err).To(gomega.HaveOccurred())
		gomega.Expect(err.Error()).To(gomega.ContainSubstring("consumerName must be provided"))
	})

	ginkgo.It("should return error when clusterProfile is nil", func() {
		clientset := fake.NewSimpleClientset()
		_, err := BuildConfigFromCP(context.TODO(), clientset, "consumer", nil)
		gomega.Expect(err).To(gomega.HaveOccurred())
		gomega.Expect(err.Error()).To(gomega.ContainSubstring("clusterProfile must not be nil"))
	})

	ginkgo.It("should return error when clusterProfile.Name is empty", func() {
		clientset := fake.NewSimpleClientset()
		cp := &v1alpha1.ClusterProfile{}
		_, err := BuildConfigFromCP(context.TODO(), clientset, "consumer", cp)
		gomega.Expect(err).To(gomega.HaveOccurred())
		gomega.Expect(err.Error()).To(gomega.ContainSubstring("clusterProfile.Name must be provided"))
	})

	ginkgo.It("should return error when no matching secret exists", func() {
		ctx := context.TODO()
		clientset := fake.NewSimpleClientset()
		// consumer namespace exists and labeled, but no secrets
		_, err := clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns", Labels: map[string]string{LabelConsumerKey: "consumer"}}}, metav1.CreateOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		cp := &v1alpha1.ClusterProfile{ObjectMeta: metav1.ObjectMeta{Name: "cp", Namespace: "default"}}
		_, err = BuildConfigFromCP(ctx, clientset, "consumer", cp)
		gomega.Expect(err).To(gomega.HaveOccurred())
		gomega.Expect(err.Error()).To(gomega.ContainSubstring("no secret found for ClusterProfile"))
	})

	ginkgo.It("should return error when matching secret lacks kubeconfig data", func() {
		ctx := context.TODO()
		clientset := fake.NewSimpleClientset()
		_, err := clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns", Labels: map[string]string{LabelConsumerKey: "consumer"}}}, metav1.CreateOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		_, err = clientset.CoreV1().Secrets("ns").Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "s",
				Namespace: "ns",
				Labels: map[string]string{
					LabelClusterProfileName: "cp",
				},
			},
			Data: map[string][]byte{},
		}, metav1.CreateOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		cp := &v1alpha1.ClusterProfile{ObjectMeta: metav1.ObjectMeta{Name: "cp", Namespace: "default"}}
		_, err = BuildConfigFromCP(ctx, clientset, "consumer", cp)
		gomega.Expect(err).To(gomega.HaveOccurred())
		gomega.Expect(err.Error()).To(gomega.ContainSubstring("does not contain kubeconfig under key"))
	})

	ginkgo.It("should return error when kubeconfig is invalid", func() {
		ctx := context.TODO()
		clientset := fake.NewSimpleClientset()
		_, err := clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns", Labels: map[string]string{LabelConsumerKey: "consumer"}}}, metav1.CreateOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		_, err = clientset.CoreV1().Secrets("ns").Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "s",
				Namespace: "ns",
				Labels: map[string]string{
					LabelClusterProfileName: "cp",
				},
			},
			Data: map[string][]byte{
				SecretDataKeyKubeconfig: []byte("not a valid kubeconfig"),
			},
		}, metav1.CreateOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		cp := &v1alpha1.ClusterProfile{ObjectMeta: metav1.ObjectMeta{Name: "cp", Namespace: "default"}}
		_, err = BuildConfigFromCP(ctx, clientset, "consumer", cp)
		gomega.Expect(err).To(gomega.HaveOccurred())
		gomega.Expect(err.Error()).To(gomega.ContainSubstring("failed to build rest.Config"))
	})
})
