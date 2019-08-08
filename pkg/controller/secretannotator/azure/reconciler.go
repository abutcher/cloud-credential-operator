package azure

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/openshift/cloud-credential-operator/pkg/controller/secretannotator/constants"
	log "github.com/sirupsen/logrus"
)

const (
	cloudCredSecretName = "azure-credentials"
	azureClientID       = "azure_client_id"
	azureClientSecret   = "azure_client_secret"
	azureSubscriptionID = "azure_subscription_id"
	azureTenantID       = "azure_tenant_id"
)

var _ reconcile.Reconciler = &ReconcileCloudCredSecret{}

type ReconcileCloudCredSecret struct {
	client.Client
	Logger log.FieldLogger
}

func NewReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileCloudCredSecret{
		Client: mgr.GetClient(),
		Logger: log.WithField("controller", constants.ControllerName),
	}
}

func Add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(constants.ControllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to cluster cloud secret
	p := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			return cloudCredSecretObjectCheck(e.MetaNew)
		},
		CreateFunc: func(e event.CreateEvent) bool {
			return cloudCredSecretObjectCheck(e.Meta)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return cloudCredSecretObjectCheck(e.Meta)
		},
	}
	err = c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForObject{}, p)
	if err != nil {
		return err
	}
	return nil
}

func cloudCredSecretObjectCheck(secret metav1.Object) bool {
	return secret.GetNamespace() == constants.CloudCredSecretNamespace && secret.GetName() == cloudCredSecretName
}

func (r *ReconcileCloudCredSecret) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	r.Logger.Info("validating cloud cred secret")

	secret := &corev1.Secret{}
	err := r.Get(context.Background(), request.NamespacedName, secret)
	if err != nil {
		r.Logger.Debugf("secret not found: %v", err)
		return reconcile.Result{}, err
	}

	err = r.validateCloudCredsSecret(secret)
	if err != nil {
		r.Logger.Errorf("error while validating cloud credentials: %v", err)
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileCloudCredSecret) validateCloudCredsSecret(secret *corev1.Secret) error {

	if _, ok := secret.Data[azureClientID]; !ok {
		r.Logger.Errorf("Couldn't fetch key containing %v from cloud cred secret", azureClientID)
		return r.updateSecretAnnotations(secret, constants.InsufficientAnnotation)
	}

	if _, ok := secret.Data[azureClientSecret]; !ok {
		r.Logger.Errorf("Couldn't fetch key containing %v from cloud cred secret", azureClientSecret)
		return r.updateSecretAnnotations(secret, constants.InsufficientAnnotation)
	}

	if _, ok := secret.Data[azureTenantID]; !ok {
		r.Logger.Errorf("Couldn't fetch key containing %v from cloud cred secret", azureTenantID)
		return r.updateSecretAnnotations(secret, constants.InsufficientAnnotation)
	}

	if _, ok := secret.Data[azureSubscriptionID]; !ok {
		r.Logger.Errorf("Couldn't fetch key containing %v from cloud cred secret", azureSubscriptionID)
		return r.updateSecretAnnotations(secret, constants.InsufficientAnnotation)
	}

	// TODO(jchaloup): find a way to dynamically check whether these creds really
	// can be used for minting (or passthrough) or whether they are useless (i.e. InsufficientAnnotation)
	r.Logger.Info("Platform is azure: allowing to mint new credentials")
	if err := r.updateSecretAnnotations(secret, constants.MintAnnotation); err != nil {
		r.Logger.Errorf("error while validating cloud credentials: %v", err)
		return err
	}

	return nil
}

func (r *ReconcileCloudCredSecret) updateSecretAnnotations(secret *corev1.Secret, value string) error {
	secretAnnotations := secret.GetAnnotations()
	if secretAnnotations == nil {
		secretAnnotations = map[string]string{}
	}

	secretAnnotations[constants.AnnotationKey] = value
	secret.SetAnnotations(secretAnnotations)

	return r.Update(context.Background(), secret)
}
