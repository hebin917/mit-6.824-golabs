package mapreduce

import "fmt"
import "log"
import "sync"

// schedule starts and waits for all tasks in the given phase (Map or Reduce).
func (mr *Master) schedule(phase jobPhase) {
	var ntasks int
	var nios int // number of inputs (for reduce) or outputs (for map)
	switch phase {
	case mapPhase:
		ntasks = len(mr.files)
		nios = mr.nReduce
	case reducePhase:
		ntasks = mr.nReduce
		nios = len(mr.files)
	}

	fmt.Printf("Schedule: %v %v tasks (%d I/Os)\n", ntasks, phase, nios)

	// All ntasks tasks have to be scheduled on workers, and only once all of
	// them have been completed successfully should the function return.
	// Remember that workers may fail, and that any given worker may finish
	// multiple tasks.
	//
	var wg sync.WaitGroup
	for i := 0; i < ntasks; i++ {
		taskNumber := i
		wg.Add(1)
		go func() {
			ok := false
			for !ok {
				worker := <-mr.registerChannel
				args := DoTaskArgs{
					JobName:       mr.jobName,
					File:          mr.files[taskNumber],
					Phase:         phase,
					TaskNumber:    taskNumber,
					NumOtherPhase: nios,
				}

				ok = call(worker, "Worker.DoTask", args, new(struct{}))
				if !ok {
					log.Printf("fail: %v %v tasks (%d I/Os)\n", taskNumber, phase, nios)
				} else {
					wg.Done()
					mr.registerChannel <- worker
				}
			}
		}()
	}
	wg.Wait()

	fmt.Printf("Schedule: %v phase done\n", phase)
}
