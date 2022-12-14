package controllers

import (
	k8sv1alpha1 "github.com/nginxinc/nginx-ingress-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func serviceForNginxIngressController(instance *k8sv1alpha1.NginxIngressController, scheme *runtime.Scheme) (*corev1.Service, error) {
	extraLabels := map[string]string{}
	extraAnnotations := map[string]string{}
	if instance.Spec.Service != nil {
		extraLabels = instance.Spec.Service.ExtraLabels
		extraAnnotations = instance.Spec.Service.ExtraAnnotations
	}

	svc := &corev1.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:        instance.Name,
			Namespace:   instance.Namespace,
			Labels:      extraLabels,
			Annotations: extraAnnotations,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:     "http",
					Protocol: "TCP",
					Port:     80,
					TargetPort: intstr.IntOrString{
						Type:   0,
						IntVal: 80,
					},
				},
				{
					Name:     "https",
					Protocol: "TCP",
					Port:     443,
					TargetPort: intstr.IntOrString{
						Type:   0,
						IntVal: 443,
					},
				},
			},
			Selector: map[string]string{"app": instance.Name},
			Type:     corev1.ServiceType(instance.Spec.ServiceType),
		},
	}

	if err := ctrl.SetControllerReference(instance, svc, scheme); err != nil {
		return nil, err
	}

	return svc, nil
}

func serviceMutateFn(svc *corev1.Service, serviceType string, labels map[string]string, annotations map[string]string) controllerutil.MutateFn {
	return func() error {
		svc.Spec.Type = corev1.ServiceType(serviceType)
		svc.Labels = labels
		svc.Annotations = annotations
		return nil
	}
}
