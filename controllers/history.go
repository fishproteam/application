package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash"
	"hash/fnv"
	"sort"

	"github.com/kr/pretty"
	apps "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	appv1beta1 "github.com/application-io/application/api/v1beta1"
)

// constructHistory ensure ControllerRevision, it return current controllerRevision and history controllerRevision
func (r *ApplicationReconciler) constructHistory(ctx context.Context, app *appv1beta1.Application) (*apps.ControllerRevision, []*apps.ControllerRevision, error) {
	logger := log.FromContext(ctx)

	var (
		err       error
		cur       *apps.ControllerRevision
		histories apps.ControllerRevisionList
	)
	if err := r.Client.List(ctx, &histories, &client.ListOptions{Namespace: app.Namespace}); err != nil {
		logger.Error(err, "failed to list controller revision list")
		return nil, nil, err
	}

	var olds, currentHistories []*apps.ControllerRevision
	for index, history := range histories.Items {
		// Add the unique label if it's not already added to the history
		// We use history name instead of computing hash, so that we don't need to worry about hash collision
		if _, ok := history.Labels[apps.DefaultDaemonSetUniqueLabelKey]; !ok {
			toUpdate := history.DeepCopy()
			toUpdate.Labels[apps.DefaultDaemonSetUniqueLabelKey] = toUpdate.Name
			err := r.Client.Update(ctx, toUpdate)
			if err != nil {
				return nil, nil, err
			}
		}
		// Compare histories with ds to separate cur and old history
		found := false
		found, err := Match(app, &history)
		if err != nil {
			return nil, nil, err
		}
		if found {
			currentHistories = append(currentHistories, &histories.Items[index])
		} else {
			olds = append(olds, &histories.Items[index])
		}
	}

	currRevision := maxRevision(olds) + 1
	switch len(currentHistories) {
	case 0:
		// Create a new history if the current one isn't found
		cur, err = r.snapshot(ctx, app, currRevision)
		if err != nil {
			return nil, nil, err
		}
	default:
		cur, err := r.dedupCurHistories(ctx, currentHistories)
		if err != nil {
			return nil, nil, err
		}
		// Update revision number if necessary
		if cur.Revision < currRevision {
			toUpdate := cur.DeepCopy()
			toUpdate.Revision = currRevision
			err := r.Client.Update(ctx, toUpdate)
			if err != nil {
				return nil, nil, err
			}
		}
	}

	return cur, olds, nil
}

// cleanupHistory clean up history by application revisionHistoryLimit
func (r *ApplicationReconciler) cleanupHistory(ctx context.Context, app *appv1beta1.Application, olds []*apps.ControllerRevision) error {

	toKeep := int(*app.Spec.RevisionHistoryLimit)
	toKill := len(olds) - toKeep
	if toKill <= 0 {
		return nil
	}

	// Clean up old history from smallest to highest revision (from oldest to newest)
	sort.Sort(historiesByRevision(olds))
	for _, history := range olds {
		if toKill <= 0 {
			break
		}
		// Clean up
		err := r.Client.Delete(ctx, history)
		if err != nil && !errors.IsNotFound(err) {
			return err
		}
		toKill--
	}
	return nil
}

// snapshot create controllerRevision by current application
func (r *ApplicationReconciler) snapshot(ctx context.Context, app *appv1beta1.Application, revision int64) (*apps.ControllerRevision, error) {
	patch, err := getPatch(app)
	if err != nil {
		return nil, err
	}
	hash := ComputeHash(app.Spec)
	name := app.Name + "-" + hash
	history := &apps.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       app.Namespace,
			Labels:          CloneAndAddLabel(app.Labels, apps.ControllerRevisionHashLabelKey, hash),
			Annotations:     app.Annotations,
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(app, appv1beta1.GroupVersion.WithKind("Application"))},
		},
		Data:     runtime.RawExtension{Raw: patch},
		Revision: revision,
	}

	err = r.Client.Create(ctx, history)
	if outerErr := err; errors.IsAlreadyExists(outerErr) {
		// TODO: Is it okay to get from historyLister?
		var existedHistory apps.ControllerRevision
		getErr := r.Client.Get(ctx, types.NamespacedName{Namespace: app.Namespace, Name: name}, &existedHistory)
		if getErr != nil {
			return nil, getErr
		}
		// Check if we already created it
		done, matchErr := Match(app, &existedHistory)
		if matchErr != nil {
			return nil, matchErr
		}
		if done {
			return &existedHistory, nil
		}
		return nil, outerErr
	}
	return history, err
}

// dedupCurHistories return the max revision controllerRevision
func (r *ApplicationReconciler) dedupCurHistories(ctx context.Context, curHistories []*apps.ControllerRevision) (*apps.ControllerRevision, error) {
	if len(curHistories) == 1 {
		return curHistories[0], nil
	}
	var maxRevision int64
	var keepCur *apps.ControllerRevision
	for _, cur := range curHistories {
		if cur.Revision >= maxRevision {
			keepCur = cur
			maxRevision = cur.Revision
		}
	}
	// Clean up duplicates and relabel pods
	for _, cur := range curHistories {
		if cur.Name == keepCur.Name {
			continue
		}
		// Remove duplicates
		err := r.Client.Delete(ctx, cur)
		if err != nil {
			return nil, err
		}
	}
	return keepCur, nil
}

// Match check if the given DaemonSet's template matches the template stored in the given history.
func Match(app *appv1beta1.Application, history *apps.ControllerRevision) (bool, error) {
	patch, err := getPatch(app)
	if err != nil {
		return false, err
	}
	return bytes.Equal(patch, history.Data.Raw), nil
}

// getPatch returns the resource section of interest
func getPatch(ds *appv1beta1.Application) ([]byte, error) {
	dsBytes, err := json.Marshal(ds)
	if err != nil {
		return nil, err
	}
	var raw map[string]interface{}
	err = json.Unmarshal(dsBytes, &raw)
	if err != nil {
		return nil, err
	}
	objCopy := make(map[string]interface{})

	// Create a patch of the DaemonSet that replaces spec.template
	spec := raw["spec"].(map[string]interface{})
	objCopy["$patch"] = "replace"
	objCopy["spec"] = spec
	patch, err := json.Marshal(objCopy)
	return patch, err
}

// maxRevision returns the max revision number of the given list of histories
func maxRevision(histories []*apps.ControllerRevision) int64 {
	max := int64(0)
	for _, history := range histories {
		if history.Revision > max {
			max = history.Revision
		}
	}
	return max
}

// ComputeHash will generate hash of controllerRevision name
func ComputeHash(appSpec appv1beta1.ApplicationSpec) string {
	data, _ := json.Marshal(appSpec)
	hash := fnv.New32a()
	DeepHashObject(hash, string(data))
	return rand.SafeEncodeString(fmt.Sprint(hash.Sum32()))
}

// DeepHashObject writes specified object to hash using the pretty library
// which follows pointers and prints actual values of the nested objects
// ensuring the hash does not change when a pointer changes.
func DeepHashObject(hasher hash.Hash, objectToWrite interface{}) {
	hasher.Reset()
	pretty.Fprintf(hasher, "%# v", objectToWrite)
}

// CloneAndAddLabel clone the given map and returns a new map with the given key and value added.
// Returns the given map, if labelKey is empty.
func CloneAndAddLabel(labels map[string]string, labelKey, labelValue string) map[string]string {
	if labelKey == "" {
		// Don't need to add a label.
		return labels
	}
	// Clone.
	newLabels := map[string]string{}
	for key, value := range labels {
		newLabels[key] = value
	}
	newLabels[labelKey] = labelValue
	return newLabels
}

//historiesByRevision implementing the sorting interface
type historiesByRevision []*apps.ControllerRevision

func (h historiesByRevision) Len() int      { return len(h) }
func (h historiesByRevision) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h historiesByRevision) Less(i, j int) bool {
	return h[i].Revision < h[j].Revision
}
