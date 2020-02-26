// Copyright 2019 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package helper

import (
	"encoding/json"
	"math"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	DeleteSlotsAnn = "delete-slots"
)

func GetDeleteSlots(set metav1.Object) (deleteSlots sets.Int32) {
	deleteSlots = sets.NewInt32()
	annotations := set.GetAnnotations()
	if annotations == nil {
		return
	}
	value, ok := annotations[DeleteSlotsAnn]
	if !ok {
		return
	}
	var slice []int32
	err := json.Unmarshal([]byte(value), &slice)
	if err != nil {
		return
	}
	deleteSlots.Insert(slice...)
	return
}

func SetDeleteSlots(set metav1.Object, deleteSlots sets.Int32) (err error) {
	annotations := set.GetAnnotations()
	if deleteSlots == nil || deleteSlots.Len() == 0 {
		// clear
		delete(annotations, DeleteSlotsAnn)
	} else {
		// set
		b, err := json.Marshal(deleteSlots.List())
		if err != nil {
			return err
		}
		if annotations == nil {
			annotations = make(map[string]string)
		}
		annotations[DeleteSlotsAnn] = string(b)
	}
	set.SetAnnotations(annotations)
	return
}

func AddDeleteSlots(set metav1.Object, deleteSlots sets.Int32) (err error) {
	currentDeleteSlots := GetDeleteSlots(set)
	return SetDeleteSlots(set, currentDeleteSlots.Union(deleteSlots))
}

// GetMaxReplicaCountAndDeleteSlots returns the max replica count and delete
// slots. The desired slots of this stateful set will be [0, replicaCount) - [delete slots].
func GetMaxReplicaCountAndDeleteSlots(replicas int32, deleteSlots sets.Int32) (int32, sets.Int32) {
	replicaCount := replicas
	deleteSlotsCopy := sets.NewInt32()
	for k := range deleteSlots {
		deleteSlotsCopy.Insert(k)
	}
	for _, deleteSlot := range deleteSlotsCopy.List() {
		if deleteSlot < replicaCount {
			replicaCount++
		} else {
			deleteSlotsCopy.Delete(deleteSlot)
		}
	}
	return replicaCount, deleteSlotsCopy
}

func GetPodOrdinals(replicas int32, set metav1.Object) sets.Int32 {
	return GetPodOrdinalsFromReplicasAndDeleteSlots(replicas, GetDeleteSlots(set))
}

func GetPodOrdinalsFromReplicasAndDeleteSlots(replicas int32, deleteSlots sets.Int32) sets.Int32 {
	maxReplicaCount, deleteSlots := GetMaxReplicaCountAndDeleteSlots(replicas, deleteSlots)
	podOrdinals := sets.NewInt32()
	for i := int32(0); i < maxReplicaCount; i++ {
		if !deleteSlots.Has(i) {
			podOrdinals.Insert(i)
		}
	}
	return podOrdinals
}

func GetMaxPodOrdinal(replicas int32, set metav1.Object) int32 {
	var max int32
	max = -1
	for k := range GetPodOrdinals(replicas, set) {
		if k > max {
			max = k
		}
	}
	return max
}

func GetMinPodOrdinal(replicas int32, set metav1.Object) int32 {
	var min int32
	min = math.MaxInt32
	for k := range GetPodOrdinals(replicas, set) {
		if k < min {
			min = k
		}
	}
	return min
}

// CalculateScaleOutPodOrdinal calculate the expected pod ordinal before the scaling out happens,
// if the currentReplicas is 3, and the target Replicas is 5.
// Then the current pod ordinal list is [0,1,3], and the delete slot list is [2,4],
// the calculated return sets would be {5,6}
func CalculateScaleOutPodOrdinal(currentReplicas, targetReplicas int32, set metav1.Object) sets.Int32 {
	expectedOrdinalSets := sets.Int32{}
	if currentReplicas >= targetReplicas {
		return expectedOrdinalSets
	}
	slots := GetDeleteSlots(set)
	maxOrdinal := GetMaxPodOrdinal(currentReplicas, set)
	for i := 1; expectedOrdinalSets.Len() < int(targetReplicas-currentReplicas); i++ {
		expectedOrdinal := maxOrdinal + int32(i)
		if !slots.Has(expectedOrdinal) {
			expectedOrdinalSets.Insert(expectedOrdinal)
		}
	}
	return expectedOrdinalSets
}
