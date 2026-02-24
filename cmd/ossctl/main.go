package main

import (
	"errors"
	"flag"
	"log"
	"os"
	"strings"

	"aihub/internal/agenthome"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

func main() {
	var (
		endpoint        = flag.String("endpoint", strings.TrimSpace(os.Getenv("AIHUB_OSS_ENDPOINT")), "OSS endpoint, e.g. https://oss-cn-hangzhou.aliyuncs.com")
		accessKeyID     = flag.String("access-key-id", strings.TrimSpace(os.Getenv("AIHUB_OSS_ACCESS_KEY_ID")), "OSS access key id")
		accessKeySecret = flag.String("access-key-secret", strings.TrimSpace(os.Getenv("AIHUB_OSS_ACCESS_KEY_SECRET")), "OSS access key secret")
		bucketName      = flag.String("bucket", strings.TrimSpace(os.Getenv("AIHUB_OSS_BUCKET")), "OSS bucket name")
		basePrefix      = flag.String("base-prefix", strings.Trim(strings.TrimSpace(os.Getenv("AIHUB_OSS_BASE_PREFIX")), "/"), "Base prefix for all objects (optional)")

		heartbeatDays = flag.Int("heartbeat-days", 7, "Heartbeat retention days (agents/heartbeats/)")
		taskDays      = flag.Int("task-days", 90, "Task retention days (tasks/)")
		apply         = flag.Bool("apply", false, "Apply/merge lifecycle rules into bucket")
	)
	flag.Parse()

	if !*apply {
		log.Fatal("no action specified (use -apply)")
	}
	if *endpoint == "" || *accessKeyID == "" || *accessKeySecret == "" || *bucketName == "" {
		log.Fatal("missing required OSS config (endpoint/access-key-id/access-key-secret/bucket)")
	}
	if *heartbeatDays < 1 || *heartbeatDays > 3650 {
		log.Fatal("invalid -heartbeat-days")
	}
	if *taskDays < 1 || *taskDays > 3650 {
		log.Fatal("invalid -task-days")
	}

	client, err := oss.New(*endpoint, *accessKeyID, *accessKeySecret)
	if err != nil {
		log.Fatalf("oss client: %v", err)
	}

	// Fetch existing lifecycle config (may not exist).
	existing, err := client.GetBucketLifecycle(*bucketName)
	if err != nil {
		var srvErr oss.ServiceError
		if errors.As(err, &srvErr) {
			// OSS returns 404 with NoSuchLifecycle when no rules exist.
			if srvErr.StatusCode == 404 && (srvErr.Code == "NoSuchLifecycle" || srvErr.Code == "NoSuchLifecycleConfiguration") {
				log.Printf("no existing lifecycle rules (bucket=%s)", *bucketName)
				existing = oss.GetBucketLifecycleResult{}
			} else {
				log.Fatalf("get lifecycle: %v", err)
			}
		} else {
			log.Fatalf("get lifecycle: %v", err)
		}
	}

	ruleHeartbeatID := "aihub_heartbeats_expire"
	ruleTasksID := "aihub_tasks_expire"

	newRules := make([]oss.LifecycleRule, 0, len(existing.Rules)+2)
	for _, r := range existing.Rules {
		if r.ID == ruleHeartbeatID || r.ID == ruleTasksID {
			continue
		}
		newRules = append(newRules, r)
	}

	heartbeatsPrefix := agenthome.JoinKey(*basePrefix, "agents/heartbeats/")
	tasksPrefix := agenthome.JoinKey(*basePrefix, "tasks/")

	newRules = append(newRules,
		oss.LifecycleRule{
			ID:     ruleHeartbeatID,
			Prefix: heartbeatsPrefix,
			Status: "Enabled",
			Expiration: &oss.LifecycleExpiration{
				Days: *heartbeatDays,
			},
		},
		oss.LifecycleRule{
			ID:     ruleTasksID,
			Prefix: tasksPrefix,
			Status: "Enabled",
			Expiration: &oss.LifecycleExpiration{
				Days: *taskDays,
			},
		},
	)

	if err := client.SetBucketLifecycle(*bucketName, newRules); err != nil {
		log.Fatalf("set lifecycle: %v", err)
	}

	log.Printf("lifecycle rules applied (heartbeats=%s days=%d, tasks=%s days=%d)", heartbeatsPrefix, *heartbeatDays, tasksPrefix, *taskDays)
}
