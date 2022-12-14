package controllers

import (
	"context"
	"fmt"

	"github.com/nginxinc/nginx-ingress-operator/controllers/scc"

	"github.com/go-logr/logr"
	k8sv1alpha1 "github.com/nginxinc/nginx-ingress-operator/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/version"
)

// checkPrerequisites creates all necessary objects before the deployment of a new Ingress Controller.
func (r *NginxIngressControllerReconciler) checkPrerequisites(log logr.Logger, instance *k8sv1alpha1.NginxIngressController) error {
	sa, err := serviceAccountForNginxIngressController(instance, r.Scheme)
	if err != nil {
		return err
	}
	existed, err := r.createIfNotExists(sa)
	if err != nil {
		return err
	}

	if !existed {
		log.Info("ServiceAccount created", "ServiceAccount.Namespace", sa.Namespace, "ServiceAccount.Name", sa.Name)
	}

	// Assign this new ServiceAccount to the ClusterRoleBinding (if is not present already)
	crb := clusterRoleBindingForNginxIngressController(clusterRoleName)

	err = r.Get(context.TODO(), types.NamespacedName{Name: clusterRoleName, Namespace: v1.NamespaceAll}, crb)
	if err != nil {
		return err
	}

	subject := subjectForServiceAccount(sa.Namespace, sa.Name)
	found := false
	for _, s := range crb.Subjects {
		if s.Name == subject.Name && s.Namespace == subject.Namespace {
			found = true
			break
		}
	}

	if !found {
		crb.Subjects = append(crb.Subjects, subject)

		err = r.Update(context.TODO(), crb)
		if err != nil {
			return err
		}
	}

	// IngressClass is available from k8s 1.18+
	minVersion, _ := version.ParseGeneric("v1.18.0")
	if RunningK8sVersion.AtLeast(minVersion) {
		ic := ingressClassForNginxIngressController(instance)
		existed, err = r.createIfNotExists(ic)
		if err != nil {
			return err
		}

		if !existed {
			log.Info("IngressClass created", "IngressClass.Name", ic.Name)
		}
	}

	if instance.Spec.DefaultSecret == "" {
		err = r.Get(context.TODO(), types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, &v1.Secret{})

		if err != nil && errors.IsNotFound(err) {
			secret, err := defaultSecretForNginxIngressController(instance, r.Scheme)
			if err != nil {
				return err
			}

			err = r.Create(context.TODO(), secret)
			if err != nil {
				return err
			}

			log.Info("Warning! A custom self-signed TLS Secret has been created for the default server. "+
				"Update your 'DefaultSecret' with your own Secret in Production",
				"Secret.Namespace", secret.Namespace, "Secret.Name", secret.Name)

		} else if err != nil {
			return err
		}
	}

	if r.SccAPIExists {
		err := scc.AddServiceAccount(r.Client, sa.Namespace, sa.Name)
		if err != nil {
			return fmt.Errorf("failed to add service account user to scc: %w", err)
		}
	}

	return nil
}

// create common resources shared by all the Ingress Controllers
func (r *NginxIngressControllerReconciler) createCommonResources(log logr.Logger) error {
	// Create ClusterRole and ClusterRoleBinding for all the NginxIngressController resources.
	var err error

	cr := clusterRoleForNginxIngressController(clusterRoleName)

	err = r.Get(context.TODO(), types.NamespacedName{Name: clusterRoleName, Namespace: v1.NamespaceAll}, cr)

	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("no previous ClusterRole found, creating a new one.")
			err = r.Create(context.TODO(), cr)
			if err != nil {
				return fmt.Errorf("error creating ClusterRole: %w", err)
			}
		} else {
			return fmt.Errorf("error getting ClusterRole: %w", err)
		}
	} else {
		// For updates in the ClusterRole permissions (eg new CRDs of the Ingress Controller).
		log.Info("previous ClusterRole found, updating.")
		cr := clusterRoleForNginxIngressController(clusterRoleName)
		err = r.Update(context.TODO(), cr)
		if err != nil {
			return fmt.Errorf("error updating ClusterRole: %w", err)
		}
	}

	crb := clusterRoleBindingForNginxIngressController(clusterRoleName)

	err = r.Get(context.TODO(), types.NamespacedName{Name: clusterRoleName, Namespace: v1.NamespaceAll}, crb)
	if err != nil && errors.IsNotFound(err) {
		log.Info("no previous ClusterRoleBinding found, creating a new one.")
		err = r.Create(context.TODO(), crb)
	}

	if err != nil {
		return fmt.Errorf("error creating ClusterRoleBinding: %w", err)
	}

	err = createKICCustomResourceDefinitions(log, r.Mgr)
	if err != nil {
		return fmt.Errorf("error creating KIC CRDs: %w", err)
	}

	if r.SccAPIExists {
		log.Info("OpenShift detected as platform.")

		err := scc.Create(r.Client, log)
		if err != nil {
			return fmt.Errorf("failed to create SecurityContextConstraints: %w", err)
		}
	}

	return nil
}
