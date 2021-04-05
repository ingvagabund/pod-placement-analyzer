package pkg

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

type PodElement struct {
	Namespace         string       `json:"namespace"`
	Kind              string       `json:"kind"`
	KindName          string       `json:"kindName"`
	PodName           string       `json:"podName"`
	Node              string       `json:"node"`
	CreationTimestamp metav1.Time  `json:creationTimestamp`
	DeletionTimestamp *metav1.Time `json:deletionTimestamp`
}

func (pe *PodElement) String() string {
	return fmt.Sprintf("ns=%v, kind=%v, kindname=%v, podname=%v, node=%v, ct=%v, dt=%v", pe.Namespace, pe.Kind, pe.KindName, pe.PodName, pe.Node, pe.CreationTimestamp, pe.DeletionTimestamp)
}

func (pe *PodElement) KindOwnerKey() string {
	return fmt.Sprintf("%v/%v/%v", pe.Namespace, pe.Kind, pe.KindName)
}

func (pe *PodElement) UniqueKey() string {
	return fmt.Sprintf("%v/%v/%v/%v", pe.Namespace, pe.Kind, pe.KindName, pe.PodName)
}

func NewPodCollector() *PodCollector {
	return &PodCollector{
		elements:         make(map[string][]*PodElement),
		podDisplacements: PodDisplacements{},
	}
}

type Edge struct {
	In, Out *PodElement
}

type PodDisplacements map[string][][]Edge

func (pd PodDisplacements) Dump() {
	for owner, chains := range pd {
		fmt.Printf("%v\n", owner)
		for _, chain := range chains {
			str := ""
			for idx, edge := range chain {
				if idx == 0 {
					str += fmt.Sprintf("%v -> %v", edge.In.UniqueKey(), edge.Out.UniqueKey())
				} else {
					str += fmt.Sprintf(" -> %v", edge.Out.UniqueKey())
				}
			}
			fmt.Printf("%v\n", str)
		}
	}
}

type PodCollector struct {
	elements map[string][]*PodElement

	podInformer cache.SharedIndexInformer

	// podDisplacements stores for each owner a list of pod replacements
	// (replacement = evicted/deleted pod replaced by a newly created one)
	podDisplacements PodDisplacements
}

func getPodElements(pod *corev1.Pod) (elements []*PodElement) {
	for _, owner := range pod.OwnerReferences {
		elements = append(elements, &PodElement{
			Namespace:         pod.Namespace,
			Kind:              owner.Kind,
			KindName:          owner.Name,
			PodName:           pod.Name,
			Node:              pod.Spec.NodeName,
			CreationTimestamp: pod.CreationTimestamp,
			DeletionTimestamp: pod.DeletionTimestamp,
		})
	}
	return
}

func (pc *PodCollector) Setup(ctx context.Context, sharedInformerFactory informers.SharedInformerFactory) {
	pc.podInformer = sharedInformerFactory.Core().V1().Pods().Informer()
	pc.podInformer.AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				pod, ok := obj.(*corev1.Pod)
				if !ok {
					return
				}
				fmt.Printf("Adding a pod: %v\n", getPodElements(pod)[0].String())
				for _, element := range getPodElements(pod) {
					pc.Record(element)
				}
			},
			DeleteFunc: func(obj interface{}) {
				pod, ok := obj.(*corev1.Pod)
				if !ok {
					return
				}
				fmt.Printf("Deleting a pod: %v\n", getPodElements(pod)[0].String())
				for _, element := range getPodElements(pod) {
					pc.Record(element)
				}
			},
		},
	)
}

func (pc *PodCollector) Run(ctx context.Context) {
	go pc.podInformer.Run(ctx.Done())
}

func (pc *PodCollector) JsonDump() (string, error) {
	bytes, err := json.Marshal(pc.elements)
	return string(bytes), err
}

func (pc *PodCollector) Import(data []byte) error {
	return json.Unmarshal(data, &pc.elements)
}

func (pc *PodCollector) Record(element *PodElement) {
	bytes, _ := json.Marshal(element)
	fmt.Println(string(bytes))

	key := element.KindOwnerKey()
	pc.elements[key] = append(pc.elements[key], element)
}

func (pc *PodCollector) ComputePodTransitions() {
	for key, podElements := range pc.elements {
		edges := map[string]*PodElement{}
		vertices := map[string]*PodElement{}
		notStart := map[string]struct{}{}
		for _, edge := range pc.computeKindOwnerPodTransitions(key, podElements) {
			vertices[edge.In.PodName] = edge.In
			vertices[edge.Out.PodName] = edge.Out
			notStart[edge.Out.PodName] = struct{}{}
			edges[edge.In.PodName] = edge.Out
			// fmt.Printf("# %v -> %v\n", edge.In.PodName, edge.Out.PodName)
		}
		pc.podDisplacements = PodDisplacements{}
		for vertex := range edges {
			if _, exists := notStart[vertex]; exists {
				continue
			}
			// fmt.Printf("s=%v\n", vertex)
			placements := []Edge{}
			for {
				if _, exists := edges[vertex]; exists {
					placements = append(placements, Edge{In: vertices[vertex], Out: vertices[edges[vertex].PodName]})
					// fmt.Printf("%v -> %v\n", vertex, edges[vertex].PodName)
					vertex = edges[vertex].PodName
				} else {
					break
				}
			}
			pc.podDisplacements[key] = append(pc.podDisplacements[key], placements)
		}
	}
}

func (pc *PodCollector) computeKindOwnerPodTransitions(kindOwner string, elements []*PodElement) (edges []Edge) {
	var sortedByCreationTimestamp []*PodElement
	var sortedByDeletionTimestamp []*PodElement

	// remove duplicates
	uniquePods := map[string]*PodElement{}
	for _, elm := range elements {
		if _, exists := uniquePods[elm.PodName]; !exists {
			uniquePods[elm.PodName] = elm
		} else {
			if !uniquePods[elm.PodName].CreationTimestamp.Equal(&elm.CreationTimestamp) {
				klog.Warningf("Detected two pods with %q name but different CreationTimestamp: %v != %v", elm.PodName, uniquePods[elm.PodName].CreationTimestamp, elm.CreationTimestamp)
			} else {
				// Add missing DeletionTimestamp
				if uniquePods[elm.PodName].DeletionTimestamp == nil && elm.DeletionTimestamp != nil {
					// fmt.Printf("%v: %v - %v\n", elm.PodName, uniquePods[elm.PodName].DeletionTimestamp, elm.DeletionTimestamp)
					uniquePods[elm.PodName] = elm
				}
			}
		}
	}

	// fmt.Printf("%v -> %v\n", len(elements), len(uniquePods))

	for _, elm := range uniquePods {
		sortedByCreationTimestamp = append(sortedByCreationTimestamp, elm)
		sortedByDeletionTimestamp = append(sortedByDeletionTimestamp, elm)
	}

	sort.Slice(sortedByCreationTimestamp, func(i, j int) bool {
		return sortedByCreationTimestamp[i].CreationTimestamp.Before(&sortedByCreationTimestamp[j].CreationTimestamp)
	})
	sort.Slice(sortedByDeletionTimestamp, func(i, j int) bool {
		if sortedByDeletionTimestamp[i].DeletionTimestamp == nil {
			return true
		}
		if sortedByDeletionTimestamp[j].DeletionTimestamp == nil {
			return false
		}
		return sortedByDeletionTimestamp[i].DeletionTimestamp.Before(sortedByDeletionTimestamp[j].DeletionTimestamp)
	})

	// for _, elm := range sortedByDeletionTimestamp {
	// 	fmt.Printf("# %v: %v\n", elm.PodName, elm.DeletionTimestamp)
	// }
	//
	// for _, elm := range sortedByCreationTimestamp {
	// 	lifespam := ""
	// 	if elm.DeletionTimestamp != nil {
	// 		lifespam = elm.DeletionTimestamp.Time.Sub(elm.CreationTimestamp.Time).String()
	// 	}
	// 	fmt.Printf("%v: %v (%v)\n", elm.PodName, elm.CreationTimestamp, lifespam)
	// }

	j := 0
	size := len(sortedByCreationTimestamp)
	for _, elm := range sortedByDeletionTimestamp {
		// No deletion ts -> no pod to append after
		if elm.DeletionTimestamp == nil {
			continue
		}
		for ; j < size; j++ {
			if sortedByCreationTimestamp[j].CreationTimestamp.Before(elm.DeletionTimestamp) {
				continue
			}
			break
		}
		if j < size {
			edges = append(edges, Edge{In: elm, Out: sortedByCreationTimestamp[j]})
			// fmt.Printf("%v -> %v\n", elm.UniqueKey(), sortedByCreationTimestamp[j].UniqueKey())
		} else {
			return
		}
		j++
	}
	return
}

func (pc *PodCollector) PodDisplacements() PodDisplacements {
	return pc.podDisplacements
}
