/*
Copyright 2022 The Kubermatic Kubernetes Platform contributors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package reconciling

import (
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilpointer "k8s.io/utils/pointer"
)

// DefaultContainer defaults all Container attributes to the same values as they would get from the Kubernetes API.
func DefaultContainer(c *corev1.Container, procMountType *corev1.ProcMountType) {
	if c.ImagePullPolicy == "" {
		c.ImagePullPolicy = corev1.PullIfNotPresent
	}
	if c.TerminationMessagePath == "" {
		c.TerminationMessagePath = corev1.TerminationMessagePathDefault
	}
	if c.TerminationMessagePolicy == "" {
		c.TerminationMessagePolicy = corev1.TerminationMessageReadFile
	}

	for idx := range c.Env {
		if c.Env[idx].ValueFrom != nil && c.Env[idx].ValueFrom.FieldRef != nil {
			if c.Env[idx].ValueFrom.FieldRef.APIVersion == "" {
				c.Env[idx].ValueFrom.FieldRef.APIVersion = "v1"
			}
		}
	}

	// This attribute was added in 1.12
	if c.SecurityContext != nil {
		c.SecurityContext.ProcMount = procMountType
	}
}

// DefaultPodSpec defaults all Container attributes to the same values as they would get from the Kubernetes API.
// In addition, it sets default PodSpec values that KKP requires in all workloads, for example appropriate security settings.
// The following KKP-specific defaults are applied:
// - SecurityContext.SeccompProfile is set to be of type `RuntimeDefault` to enable seccomp isolation if not set.
func DefaultPodSpec(oldPodSpec, newPodSpec corev1.PodSpec) (corev1.PodSpec, error) {
	// make sure to keep the old procmount types in case a creator overrides the entire PodSpec
	initContainerProcMountType := map[string]*corev1.ProcMountType{}
	containerProcMountType := map[string]*corev1.ProcMountType{}
	for _, container := range oldPodSpec.InitContainers {
		if container.SecurityContext != nil {
			initContainerProcMountType[container.Name] = container.SecurityContext.ProcMount
		}
	}
	for _, container := range oldPodSpec.Containers {
		if container.SecurityContext != nil {
			containerProcMountType[container.Name] = container.SecurityContext.ProcMount
		}
	}

	for idx, container := range newPodSpec.InitContainers {
		DefaultContainer(&newPodSpec.InitContainers[idx], initContainerProcMountType[container.Name])
	}

	for idx, container := range newPodSpec.Containers {
		DefaultContainer(&newPodSpec.Containers[idx], containerProcMountType[container.Name])
	}

	for idx, vol := range newPodSpec.Volumes {
		if vol.VolumeSource.Secret != nil && vol.VolumeSource.Secret.DefaultMode == nil {
			newPodSpec.Volumes[idx].Secret.DefaultMode = utilpointer.Int32(corev1.SecretVolumeSourceDefaultMode)
		}
		if vol.VolumeSource.ConfigMap != nil && vol.VolumeSource.ConfigMap.DefaultMode == nil {
			newPodSpec.Volumes[idx].ConfigMap.DefaultMode = utilpointer.Int32(corev1.ConfigMapVolumeSourceDefaultMode)
		}
	}

	// set KKP specific defaults for every Pod created by it

	if newPodSpec.SecurityContext == nil {
		newPodSpec.SecurityContext = &corev1.PodSecurityContext{}
	}

	if newPodSpec.SecurityContext.SeccompProfile == nil {
		newPodSpec.SecurityContext.SeccompProfile = &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		}
	}

	return newPodSpec, nil
}

// DefaultDeployment defaults all Deployment attributes to the same values as they would get from the Kubernetes API.
// In addition, the Deployment's PodSpec template gets defaulted with KKP-specific values (see DefaultPodSpec for details).
func DefaultDeployment(reconciler DeploymentReconciler) DeploymentReconciler {
	return func(d *appsv1.Deployment) (*appsv1.Deployment, error) {
		old := d.DeepCopy()

		d, err := reconciler(d)
		if err != nil {
			return nil, err
		}

		if d.Spec.Strategy.Type == "" {
			d.Spec.Strategy.Type = appsv1.RollingUpdateDeploymentStrategyType

			if d.Spec.Strategy.RollingUpdate == nil {
				d.Spec.Strategy.RollingUpdate = &appsv1.RollingUpdateDeployment{
					MaxSurge: &intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 1,
					},
					MaxUnavailable: &intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 0,
					},
				}
			}
		}

		d.Spec.Template.Spec, err = DefaultPodSpec(old.Spec.Template.Spec, d.Spec.Template.Spec)
		if err != nil {
			return nil, err
		}

		return d, nil
	}
}

// DefaultStatefulSet defaults all StatefulSet attributes to the same values as they would get from the Kubernetes API.
// In addition, the StatefulSet's PodSpec template gets defaulted with KKP-specific values (see DefaultPodSpec for details).
func DefaultStatefulSet(reconciler StatefulSetReconciler) StatefulSetReconciler {
	return func(ss *appsv1.StatefulSet) (*appsv1.StatefulSet, error) {
		old := ss.DeepCopy()

		ss, err := reconciler(ss)
		if err != nil {
			return nil, err
		}

		ss.Spec.Template.Spec, err = DefaultPodSpec(old.Spec.Template.Spec, ss.Spec.Template.Spec)
		if err != nil {
			return nil, err
		}

		return ss, nil
	}
}

// DefaultDaemonSet defaults all DaemonSet attributes to the same values as they would get from the Kubernetes API.
// In addition, the DaemonSet's PodSpec template gets defaulted with KKP-specific values (see DefaultPodSpec for details).
func DefaultDaemonSet(reconciler DaemonSetReconciler) DaemonSetReconciler {
	return func(ds *appsv1.DaemonSet) (*appsv1.DaemonSet, error) {
		old := ds.DeepCopy()

		ds, err := reconciler(ds)
		if err != nil {
			return nil, err
		}

		ds.Spec.Template.Spec, err = DefaultPodSpec(old.Spec.Template.Spec, ds.Spec.Template.Spec)
		if err != nil {
			return nil, err
		}

		return ds, nil
	}
}

// DefaultCronJob defaults all CronJob attributes to the same values as they would get from the Kubernetes API.
// In addition, the CronJob's PodSpec template gets defaulted with KKP-specific values (see DefaultPodSpec for details).
func DefaultCronJob(reconciler CronJobReconciler) CronJobReconciler {
	return func(cj *batchv1.CronJob) (*batchv1.CronJob, error) {
		old := cj.DeepCopy()

		cj, err := reconciler(cj)
		if err != nil {
			return nil, err
		}

		cj.Spec.JobTemplate.Spec.Template.Spec, err = DefaultPodSpec(old.Spec.JobTemplate.Spec.Template.Spec, cj.Spec.JobTemplate.Spec.Template.Spec)
		if err != nil {
			return nil, err
		}

		return cj, nil
	}
}
