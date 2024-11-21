package controller

import (
	"fmt"
	"math/rand"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	vahkanev1 "github.com/ushitora-anqou/vahkane/api/v1"
	discord "github.com/ushitora-anqou/vahkane/internal/discord"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type reporter struct{}

func (g reporter) Errorf(format string, args ...any) {
	Fail(fmt.Sprintf(format, args...))
}

func (g reporter) Fatalf(format string, args ...any) {
	Fail(fmt.Sprintf(format, args...))
}

var usedResourceNames = make(map[string]bool)

func gensym(prefix string) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	buf := make([]byte, 8)
	for i := range buf {
		buf[i] = letters[rand.Intn(len(letters))]
	}
	name := fmt.Sprintf("%s-%s", prefix, string(buf))
	if usedResourceNames[name] {
		return gensym(prefix)
	}
	usedResourceNames[name] = true
	return name
}

var _ = Describe("DiscordInteraction Controller", func() {
	Context("When reconciling a resource", func() {
		var mockCtrl *gomock.Controller
		var discordClient *discord.MockClient
		var ns, nsController string
		var reconciler *DiscordInteractionReconciler

		BeforeEach(func(ctx SpecContext) {
			var err error

			var t reporter
			mockCtrl = gomock.NewController(t)
			defer mockCtrl.Finish()

			discordClient = discord.NewMockClient(mockCtrl)

			ns = gensym("ns")
			err = k8sClient.Create(
				ctx,
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}},
			)
			Expect(err).NotTo(HaveOccurred())

			nsController = gensym("ns")
			err = k8sClient.Create(
				ctx,
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsController}},
			)
			Expect(err).NotTo(HaveOccurred())

			reconciler = NewDiscordInteractionReconciler(k8sClient, scheme.Scheme, nsController, discordClient)
		})

		It("should successfully reconcile the resource", func(ctx SpecContext) {
			var err error

			guildID := "test-guild"
			diName := "test"
			diNamespacedName := types.NamespacedName{Name: diName, Namespace: ns}

			var di vahkanev1.DiscordInteraction
			di.SetName(diName)
			di.SetNamespace(ns)
			di.Spec.GuildID = guildID
			di.Spec.Actions = []vahkanev1.DiscordInteractionAction{}
			di.Spec.Commands = []string{`
a:
  - b
  - c: d`,
				`- e: f`}
			err = k8sClient.Create(ctx, &di)
			Expect(err).NotTo(HaveOccurred())

			// The first reconciliation should attach a finalizer and requeue.
			res, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: diNamespacedName})
			Expect(err).NotTo(HaveOccurred())
			Expect(res.Requeue).To(BeTrue())
			err = k8sClient.Get(ctx, diNamespacedName, &di)
			Expect(err).NotTo(HaveOccurred())
			Expect(controllerutil.ContainsFinalizer(&di, finalizerDiscordInteraction)).To(BeTrue())

			// The second reconciliation should do the job.
			discordClient.EXPECT().
				GetGuildCommands(gomock.Any(), gomock.Eq(guildID)).
				Return([]map[string]interface{}{{"id": "test-id"}}, nil)
			discordClient.EXPECT().
				DeleteGuildCommand(gomock.Any(), gomock.Eq(guildID), gomock.Eq("test-id")).
				Return(nil)
			discordClient.EXPECT().
				RegisterGuildCommand(gomock.Any(), gomock.Eq(guildID), gomock.Eq(`{"a":["b",{"c":"d"}]}`)).
				Return(nil)
			discordClient.EXPECT().
				RegisterGuildCommand(gomock.Any(), gomock.Eq(guildID), gomock.Eq(`[{"e":"f"}]`)).
				Return(nil)
			res, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: diNamespacedName})
			Expect(err).NotTo(HaveOccurred())
			err = k8sClient.Get(ctx, diNamespacedName, &di)
			Expect(err).NotTo(HaveOccurred())
			Expect(di.GetLabels()[LabelKeyDiscordGuildID]).To(Equal(guildID))
			Expect(di.GetAnnotations()[annotKeyCommands]).To(Equal("6b5f7c38fb283380494a352114936fdb4872873cac11484f90430b36"))

			// The third reconciliation should do nothing.
			res, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: diNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			// If Reconcile is called after di is deleted, the finalizer should be removed.
			discordClient.EXPECT().
				GetGuildCommands(gomock.Any(), gomock.Eq(guildID)).
				Return([]map[string]interface{}{{"id": "test-id"}}, nil)
			discordClient.EXPECT().
				DeleteGuildCommand(gomock.Any(), gomock.Eq(guildID), gomock.Eq("test-id")).
				Return(nil)
			err = k8sClient.Delete(ctx, &di)
			Expect(err).NotTo(HaveOccurred())
			res, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: diNamespacedName})
			Expect(err).NotTo(HaveOccurred())
			err = k8sClient.Get(ctx, diNamespacedName, &di)
			Expect(err).To(HaveOccurred())
			Expect(k8serrors.IsNotFound(err)).To(BeTrue())
		})
	})
})
