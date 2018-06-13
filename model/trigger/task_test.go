package trigger

import (
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/alertrecord"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

func TestTaskTriggers(t *testing.T) {
	suite.Run(t, &taskSuite{})
}

type taskSuite struct {
	event event.EventLogEntry
	data  *event.TaskEventData
	task  task.Task
	subs  []event.Subscription

	t *taskTriggers

	suite.Suite
}

func (s *taskSuite) SetupSuite() {
	s.Require().Implements((*eventHandler)(nil), &taskTriggers{})
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
}

func (s *taskSuite) SetupTest() {
	s.NoError(db.ClearCollections(event.AllLogCollection, task.Collection, task.OldCollection, event.SubscriptionsCollection, alertrecord.Collection, testresult.Collection))
	startTime := time.Now().Truncate(time.Millisecond)

	s.task = task.Task{
		Id:                  "test",
		Version:             "test_version_id",
		BuildId:             "test_build_id",
		BuildVariant:        "test_build_variant",
		DistroId:            "test_distro_id",
		Project:             "test_project",
		DisplayName:         "test-display-name",
		StartTime:           startTime,
		FinishTime:          startTime.Add(10 * time.Minute),
		RevisionOrderNumber: 1,
		Requester:           evergreen.RepotrackerVersionRequester,
	}
	s.NoError(s.task.Insert())

	s.data = &event.TaskEventData{
		Status: evergreen.TaskStarted,
	}
	s.event = event.EventLogEntry{
		ResourceType: event.ResourceTypeTask,
		ResourceId:   "test",
		Data:         s.data,
	}

	s.subs = []event.Subscription{
		{
			ID:      bson.NewObjectId(),
			Type:    event.ResourceTypeTask,
			Trigger: "outcome",
			Selectors: []event.Selector{
				{
					Type: "id",
					Data: s.event.ResourceId,
				},
			},
			Subscriber: event.Subscriber{
				Type: event.EvergreenWebhookSubscriberType,
				Target: &event.WebhookSubscriber{
					URL:    "http://example.com/2",
					Secret: []byte("secret"),
				},
			},
			Owner: "someone",
		},
		{
			ID:      bson.NewObjectId(),
			Type:    event.ResourceTypeTask,
			Trigger: "success",
			Selectors: []event.Selector{
				{
					Type: "id",
					Data: s.event.ResourceId,
				},
			},
			Subscriber: event.Subscriber{
				Type: event.EvergreenWebhookSubscriberType,
				Target: &event.WebhookSubscriber{
					URL:    "http://example.com/2",
					Secret: []byte("secret"),
				},
			},
			Owner: "someone",
		},
		{
			ID:      bson.NewObjectId(),
			Type:    event.ResourceTypeTask,
			Trigger: "failure",
			Selectors: []event.Selector{
				{
					Type: "id",
					Data: s.event.ResourceId,
				},
			},
			Subscriber: event.Subscriber{
				Type: event.EvergreenWebhookSubscriberType,
				Target: &event.WebhookSubscriber{
					URL:    "http://example.com/2",
					Secret: []byte("secret"),
				},
			},
			Owner: "someone",
		},
	}

	for i := range s.subs {
		s.NoError(s.subs[i].Upsert())
	}

	ui := &evergreen.UIConfig{
		Url: "https://evergreen.mongodb.com",
	}
	s.NoError(ui.Set())

	s.t = makeTaskTriggers().(*taskTriggers)
	s.t.event = &s.event
	s.t.data = s.data
	s.t.task = &s.task
	s.t.uiConfig = *ui
}

func (s *taskSuite) TestAllTriggers() {
	n, err := NotificationsFromEvent(&s.event)
	s.NoError(err)
	s.Len(n, 0)

	s.task.Status = evergreen.TaskSucceeded
	s.data.Status = evergreen.TaskSucceeded
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))

	n, err = NotificationsFromEvent(&s.event)
	s.NoError(err)
	s.Len(n, 2)

	s.task.Status = evergreen.TaskFailed
	s.data.Status = evergreen.TaskFailed
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))

	n, err = NotificationsFromEvent(&s.event)
	s.NoError(err)
	s.Len(n, 2)
}

func (s *taskSuite) TestSuccess() {
	n, err := s.t.taskSuccess(&s.subs[1])
	s.NoError(err)
	s.Nil(n)

	s.data.Status = evergreen.TaskFailed
	n, err = s.t.taskSuccess(&s.subs[1])
	s.NoError(err)
	s.Nil(n)

	s.data.Status = evergreen.TaskSucceeded
	n, err = s.t.taskSuccess(&s.subs[1])
	s.NoError(err)
	s.NotNil(n)
}

func (s *taskSuite) TestFailure() {
	n, err := s.t.taskFailure(&s.subs[2])
	s.NoError(err)
	s.Nil(n)

	s.data.Status = evergreen.TaskSucceeded
	n, err = s.t.taskFailure(&s.subs[2])
	s.NoError(err)
	s.Nil(n)

	s.data.Status = evergreen.TaskFailed
	n, err = s.t.taskFailure(&s.subs[2])
	s.NoError(err)
	s.NotNil(n)
}

func (s *taskSuite) TestOutcome() {
	s.data.Status = evergreen.TaskStarted
	n, err := s.t.taskOutcome(&s.subs[0])
	s.NoError(err)
	s.Nil(n)

	s.data.Status = evergreen.TaskSucceeded
	n, err = s.t.taskOutcome(&s.subs[0])
	s.NoError(err)
	s.NotNil(n)

	s.data.Status = evergreen.TaskFailed
	n, err = s.t.taskOutcome(&s.subs[0])
	s.NoError(err)
	s.NotNil(n)
}

func (s *taskSuite) TestFirstFailureInVersion() {
	s.data.Status = evergreen.TaskFailed
	s.task.Status = evergreen.TaskFailed
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))

	n, err := s.t.taskFirstFailureInVersion(&s.subs[2])
	s.NoError(err)
	s.NotNil(n)

	// rerun that fails should not do anything
	n, err = s.t.taskFirstFailureInVersion(&s.subs[2])
	s.NoError(err)
	s.Nil(n)

	// subsequent runs with other tasks should not do anything
	s.task.Id = "task2"
	s.NoError(s.task.Insert())
	n, err = s.t.taskFirstFailureInVersion(&s.subs[2])
	s.NoError(err)
	s.Nil(n)

	// subsequent runs with other tasks in other builds should not do anything
	s.task.BuildId = "test2"
	s.task.BuildVariant = "test2"
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))
	n, err = s.t.taskFirstFailureInVersion(&s.subs[2])
	s.NoError(err)
	s.Nil(n)

	// subsequent runs with other tasks in other versions should still generate
	s.task.Version = "test2"
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))
	n, err = s.t.taskFirstFailureInVersion(&s.subs[2])
	s.NoError(err)
	s.NotNil(n)
}

func (s *taskSuite) TestFirstFailureInBuild() {
	s.data.Status = evergreen.TaskFailed
	s.task.Status = evergreen.TaskFailed
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))

	n, err := s.t.taskFirstFailureInBuild(&s.subs[2])
	s.NoError(err)
	s.NotNil(n)

	// rerun that fails should not do anything
	n, err = s.t.taskFirstFailureInBuild(&s.subs[2])
	s.NoError(err)
	s.Nil(n)

	// subsequent runs with other tasks should not do anything
	s.task.Id = "task2"
	s.NoError(s.task.Insert())
	n, err = s.t.taskFirstFailureInBuild(&s.subs[2])
	s.NoError(err)
	s.Nil(n)

	// subsequent runs with other tasks in other builds should generate
	s.task.BuildId = "test2"
	s.task.BuildVariant = "test2"
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))
	n, err = s.t.taskFirstFailureInBuild(&s.subs[2])
	s.NoError(err)
	s.NotNil(n)

	// subsequent runs with other tasks in other versions should generate
	s.task.Version = "test2"
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))
	n, err = s.t.taskFirstFailureInBuild(&s.subs[2])
	s.NoError(err)
	s.NotNil(n)
}

func (s *taskSuite) TestFirstFailureInVersionWithName() {
	s.data.Status = evergreen.TaskFailed
	s.task.Status = evergreen.TaskFailed
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))

	n, err := s.t.taskFirstFailureInVersionWithName(&s.subs[2])
	s.NoError(err)
	s.NotNil(n)

	// rerun that fails should not do anything
	n, err = s.t.taskFirstFailureInVersionWithName(&s.subs[2])
	s.NoError(err)
	s.Nil(n)

	// subsequent runs with other tasks should not do anything
	s.task.Id = "task2"
	s.NoError(s.task.Insert())
	n, err = s.t.taskFirstFailureInVersionWithName(&s.subs[2])
	s.NoError(err)
	s.Nil(n)

	// subsequent runs with other tasks in other builds should not generate
	s.task.BuildId = "test2"
	s.task.BuildVariant = "test2"
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))
	n, err = s.t.taskFirstFailureInVersionWithName(&s.subs[2])
	s.NoError(err)
	s.Nil(n)

	// subsequent runs in other versions should generate
	s.task.Version = "test2"
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))
	n, err = s.t.taskFirstFailureInVersionWithName(&s.subs[2])
	s.NoError(err)
	s.NotNil(n)
}

func (s *taskSuite) TestRegression() {
	s.data.Status = evergreen.TaskFailed
	s.task.Status = evergreen.TaskFailed
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))

	// brand new task fails should generate
	n, err := s.t.taskFirstFailureInVersionWithName(&s.subs[2])
	s.NoError(err)
	s.NotNil(n)

	// next fail shouldn't generate
	s.task.RevisionOrderNumber = 2
	s.task.Id = "test2"
	s.NoError(s.task.Insert())

	n, err = s.t.taskFirstFailureInVersionWithName(&s.subs[2])
	s.NoError(err)
	s.Nil(n)

	// successful task shouldn't generate
	s.task.Id = "test3"
	s.task.Version = "test3"
	s.task.BuildId = "test3"
	s.task.RevisionOrderNumber = 3
	s.task.Status = evergreen.TaskSucceeded
	s.data.Status = evergreen.TaskSucceeded
	s.NoError(s.task.Insert())

	n, err = s.t.taskFirstFailureInVersionWithName(&s.subs[2])
	s.NoError(err)
	s.Nil(n)

	// formerly succeeding task should generate
	s.task.Id = "test4"
	s.task.Version = "test4"
	s.task.BuildId = "test4"
	s.task.RevisionOrderNumber = 4
	s.task.Status = evergreen.TaskFailed
	s.data.Status = evergreen.TaskFailed
	s.NoError(s.task.Insert())

	n, err = s.t.taskFirstFailureInVersionWithName(&s.subs[2])
	s.NoError(err)
	s.NotNil(n)

	// Don't renotify if it's recent
	s.task.Id = "test5"
	s.task.Version = "test5"
	s.task.BuildId = "test5"
	s.task.RevisionOrderNumber = 5
	s.NoError(s.task.Insert())
	n, err = s.t.taskFirstFailureInVersionWithName(&s.subs[2])
	s.NoError(err)
	s.NotNil(n)

	// already failing task should renotify if the task failed more than
	// 2 days ago
	oldTime := s.task.FinishTime
	s.task.FinishTime = oldTime.Add(-3 * 24 * time.Hour)
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))
	s.task.Id = "test6"
	s.task.Version = "test6"
	s.task.BuildId = "test6"
	s.task.RevisionOrderNumber = 6
	s.NoError(s.task.Insert())

	n, err = s.t.taskFirstFailureInVersionWithName(&s.subs[2])
	s.NoError(err)
	s.NotNil(n)
	s.task.FinishTime = oldTime
	s.NoError(db.Update(task.Collection, bson.M{"_id": s.task.Id}, &s.task))

	// if regression was trigged after an older success, we should generate
	s.task.Id = "test7"
	s.task.Version = "test7"
	s.task.BuildId = "test7"
	s.task.RevisionOrderNumber = 7
	s.task.Status = evergreen.TaskSucceeded
	s.NoError(s.task.Insert())
	s.task.Id = "test8"
	s.task.Version = "test8"
	s.task.BuildId = "test8"
	s.task.RevisionOrderNumber = 8
	s.task.Status = evergreen.TaskFailed
	s.NoError(s.task.Insert())
	n, err = s.t.taskFirstFailureInVersionWithName(&s.subs[2])
	s.NoError(err)
	s.NotNil(n)

	// suppose we reran task test4, it shouldn't generate because we already
	// alerted on it
	task4 := &task.Task{}
	s.NoError(db.FindOneQ(task.Collection, db.Query(bson.M{"_id": "test4"}), task4))
	s.NotZero(*task4)
	task4.Execution = 1
	n, err = s.t.taskFirstFailureInVersionWithName(&s.subs[2])
	s.NoError(err)
	s.Nil(n)
}

func (s *taskSuite) makeTask(n int, taskStatus string) {
	s.task.Id = fmt.Sprintf("task_%d", n)
	s.task.Version = fmt.Sprintf("version_%d", n)
	s.task.BuildId = fmt.Sprintf("build_id_%d", n)
	s.task.RevisionOrderNumber = n
	s.task.Status = taskStatus
	s.data.Status = taskStatus
	s.event.ResourceId = s.task.Id
	s.NoError(s.task.Insert())
}

func (s *taskSuite) makeTest(n, execution int, testName, testStatus string) {
	if len(testName) == 0 {
		testName = "test_0"
	}
	results := testresult.TestResult{
		ID:        bson.NewObjectId(),
		TestFile:  testName,
		TaskID:    s.task.Id,
		Execution: execution,
		Status:    testStatus,
	}
	s.NoError(results.Insert())
}

func (s *taskSuite) tryDoubleTrigger(shouldGenerate bool) {
	n, err := s.t.taskRegressionByTest(&s.subs[2])
	s.NoError(err)
	s.Equal(shouldGenerate, n != nil)

	// triggering the notification again should not generate anything
	n, err = s.t.taskRegressionByTest(&s.subs[2])
	s.NoError(err)
	s.Nil(n)
}

func (s *taskSuite) TestRegressionByTest() {
	s.NoError(db.ClearCollections(task.Collection, testresult.Collection))

	// brand new task that succeeds should not generate
	s.makeTask(1, evergreen.TaskSucceeded)
	s.makeTest(1, 0, "", evergreen.TestSucceededStatus)

	s.tryDoubleTrigger(false)

	s.NoError(db.ClearCollections(task.Collection, testresult.Collection))

	// brand new test fails should generate
	s.makeTask(1, evergreen.TaskFailed)
	s.makeTest(1, 0, "", evergreen.TestFailedStatus)

	s.tryDoubleTrigger(true)

	// next fail with same test shouldn't generate
	s.makeTask(2, evergreen.TaskFailed)
	s.makeTest(2, 0, "", evergreen.TestFailedStatus)

	s.tryDoubleTrigger(false)

	// but if we add a new failed test, it should notify
	s.makeTest(2, 0, "test_1", evergreen.TestFailedStatus)
	s.tryDoubleTrigger(true)

	// successful tasks shouldn't generate
	s.makeTask(3, evergreen.TaskSucceeded)
	s.makeTest(3, 0, "", evergreen.TestSucceededStatus)
	s.makeTest(3, 0, "test_1", evergreen.TestSucceededStatus)

	s.tryDoubleTrigger(false)

	// transition to failure
	s.makeTask(4, evergreen.TaskFailed)
	s.makeTest(4, 0, "", evergreen.TestFailedStatus)
	s.makeTest(4, 0, "test_1", evergreen.TestSucceededStatus)

	s.tryDoubleTrigger(true)

	// Remove a successful test, but we already notified on the remaining
	// failed test
	s.makeTask(5, evergreen.TaskFailed)
	s.makeTest(5, 0, "", evergreen.TestFailedStatus)

	s.tryDoubleTrigger(false)

	// insert a couple of successful tasks
	s.makeTask(6, evergreen.TaskSucceeded)
	s.makeTest(6, 0, "", evergreen.TestSucceededStatus)

	s.makeTask(7, evergreen.TaskSucceeded)
	s.makeTest(7, 0, "", evergreen.TestSucceededStatus)

	s.makeTask(8, evergreen.TaskSucceeded)
	s.makeTest(8, 0, "", evergreen.TestSucceededStatus)

	// now simulate a rerun of task6 failing
	task7, err := task.FindOneIdOldOrNew("task_7", 0)
	s.NoError(err)
	s.NotNil(task7)
	s.NoError(task7.Archive())
	task7.Status = evergreen.TaskFailed
	task7.Execution = 1
	s.event.ResourceId = task7.Id
	s.NoError(db.Update(task.Collection, bson.M{"_id": task7.Id}, &task7))

	s.task = *task7
	s.makeTest(7, 1, "", evergreen.TestFailedStatus)
	s.tryDoubleTrigger(true)

	// make it fail again; it shouldn't generate
	s.NoError(task7.Archive())
	task7.Status = evergreen.TaskFailed
	task7.Execution = 2
	s.event.ResourceId = task7.Id
	s.NoError(db.Update(task.Collection, bson.M{"_id": task7.Id}, &task7))
	s.makeTest(7, 2, "", evergreen.TestFailedStatus)

	s.tryDoubleTrigger(false)

	// no tests, but system fail should generate
	s.makeTask(9, evergreen.TaskSystemFailed)
	s.tryDoubleTrigger(true)

	// but not in subsequent task
	s.makeTask(10, evergreen.TaskSystemFailed)
	s.tryDoubleTrigger(false)

	// Change in failure type should notify -> fail
	s.makeTask(11, evergreen.TaskFailed)
	s.makeTest(11, 0, "", evergreen.TestFailedStatus)
	s.tryDoubleTrigger(true)

	// Change in failure type should notify -> system fail
	s.makeTask(12, evergreen.TaskSystemFailed)
	s.makeTest(12, 0, "", evergreen.TestFailedStatus)
	s.tryDoubleTrigger(true)

	// TaskFailed with no tests should generate
	s.makeTask(13, evergreen.TaskFailed)
	s.tryDoubleTrigger(true)

	// but not in a subsequent task
	s.makeTask(14, evergreen.TaskFailed)
	s.tryDoubleTrigger(true)
}

func TestIsTestRegression(t *testing.T) {
	assert := assert.New(t)

	assert.True(isTestRegression(evergreen.TestSkippedStatus, evergreen.TestFailedStatus))
	assert.False(isTestRegression(evergreen.TestSkippedStatus, evergreen.TestSilentlyFailedStatus))
	assert.False(isTestRegression(evergreen.TestSkippedStatus, evergreen.TestSkippedStatus))
	assert.False(isTestRegression(evergreen.TestSkippedStatus, evergreen.TestSucceededStatus))

	assert.False(isTestRegression(evergreen.TestFailedStatus, evergreen.TestFailedStatus))
	assert.False(isTestRegression(evergreen.TestFailedStatus, evergreen.TestSilentlyFailedStatus))
	assert.False(isTestRegression(evergreen.TestFailedStatus, evergreen.TestSkippedStatus))
	assert.False(isTestRegression(evergreen.TestFailedStatus, evergreen.TestSucceededStatus))

	assert.True(isTestRegression(evergreen.TestSucceededStatus, evergreen.TestFailedStatus))
	assert.False(isTestRegression(evergreen.TestSucceededStatus, evergreen.TestSilentlyFailedStatus))
	assert.False(isTestRegression(evergreen.TestSucceededStatus, evergreen.TestSkippedStatus))
	assert.False(isTestRegression(evergreen.TestSucceededStatus, evergreen.TestSucceededStatus))

	assert.True(isTestRegression(evergreen.TestSilentlyFailedStatus, evergreen.TestFailedStatus))
	assert.False(isTestRegression(evergreen.TestSilentlyFailedStatus, evergreen.TestSilentlyFailedStatus))
	assert.False(isTestRegression(evergreen.TestSilentlyFailedStatus, evergreen.TestSkippedStatus))
	assert.False(isTestRegression(evergreen.TestSilentlyFailedStatus, evergreen.TestSucceededStatus))
}

func TestIsTaskRegression(t *testing.T) {
	assert := assert.New(t)

	assert.False(isTaskRegression(evergreen.TaskSucceeded, evergreen.TaskSucceeded))
	assert.True(isTaskRegression(evergreen.TaskSucceeded, evergreen.TaskSystemFailed))
	assert.True(isTaskRegression(evergreen.TaskSucceeded, evergreen.TaskFailed))
	assert.True(isTaskRegression(evergreen.TaskSucceeded, evergreen.TaskTestTimedOut))

	assert.False(isTaskRegression(evergreen.TaskSystemFailed, evergreen.TaskSucceeded))
	assert.False(isTaskRegression(evergreen.TaskSystemFailed, evergreen.TaskSystemFailed))
	assert.True(isTaskRegression(evergreen.TaskSystemFailed, evergreen.TaskFailed))
	assert.True(isTaskRegression(evergreen.TaskSystemFailed, evergreen.TaskTestTimedOut))

	assert.False(isTaskRegression(evergreen.TaskFailed, evergreen.TaskSucceeded))
	assert.True(isTaskRegression(evergreen.TaskFailed, evergreen.TaskSystemFailed))
	assert.False(isTaskRegression(evergreen.TaskFailed, evergreen.TaskFailed))
	assert.True(isTaskRegression(evergreen.TaskFailed, evergreen.TaskTestTimedOut))
}
