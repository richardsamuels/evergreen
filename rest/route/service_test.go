package route

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	serviceModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"gopkg.in/mgo.v2/bson"
)

func TestHostParseAndValidate(t *testing.T) {
	Convey("With a hostGetHandler and request", t, func() {
		testStatus := "testStatus"
		hgh := &hostGetHandler{}
		hgh, ok := hgh.Factory().(*hostGetHandler)
		So(ok, ShouldBeTrue)
		u := url.URL{
			RawQuery: fmt.Sprintf("status=%s", testStatus),
		}
		r := http.Request{
			URL: &u,
		}
		ctx := context.Background()

		Convey("parsing request should fetch status", func() {
			err := hgh.Parse(ctx, &r)
			So(err, ShouldBeNil)
			So(ok, ShouldBeTrue)
			So(hgh.status, ShouldEqual, testStatus)
		})
	})
}

func TestHostPaginator(t *testing.T) {
	numHostsInDB := 300
	Convey("When paginating with a Connector", t, func() {
		serviceContext := data.MockConnector{
			URL: "http://evergreen.example.net",
		}
		Convey("and there are hosts to be found", func() {
			cachedHosts := []host.Host{}
			for i := 0; i < numHostsInDB; i++ {
				nextHost := host.Host{
					Id: fmt.Sprintf("host%d", i),
				}
				cachedHosts = append(cachedHosts, nextHost)
			}
			serviceContext.MockHostConnector.CachedHosts = cachedHosts
			Convey("then finding a key in the middle of the set should produce"+
				" a full next and previous page and a full set of models", func() {
				hostToStartAt := 100
				limit := 100
				expectedHosts := []model.Model{}
				for i := hostToStartAt; i < hostToStartAt+limit; i++ {
					nextModelHost := &model.APIHost{
						Id:      model.ToAPIString(fmt.Sprintf("host%d", i)),
						HostURL: model.ToAPIString(""),
						Distro: model.DistroInfo{
							Id:       model.ToAPIString(""),
							Provider: model.ToAPIString(""),
						},
						StartedBy: model.ToAPIString(""),
						Type:      model.ToAPIString(""),
						User:      model.ToAPIString(""),
						Status:    model.ToAPIString(""),
					}
					expectedHosts = append(expectedHosts, nextModelHost)
				}
				expectedPages := &gimlet.ResponsePages{
					Next: &gimlet.Page{
						Key:             fmt.Sprintf("host%d", hostToStartAt+limit),
						Limit:           limit,
						Relation:        "next",
						BaseURL:         serviceContext.GetURL(),
						KeyQueryParam:   "host_id",
						LimitQueryParam: "limit",
					},
				}
				handler := &hostGetHandler{
					sc:    &serviceContext,
					key:   cachedHosts[hostToStartAt].Id,
					limit: limit,
				}
				validatePaginatedResponse(t, handler, expectedHosts, expectedPages)
			})
			Convey("then finding a key in the near the end of the set should produce"+
				" a limited next and full previous page and a full set of models", func() {
				hostToStartAt := 150
				limit := 100
				expectedHosts := []model.Model{}
				for i := hostToStartAt; i < hostToStartAt+limit; i++ {
					nextModelHost := &model.APIHost{
						Id:      model.ToAPIString(fmt.Sprintf("host%d", i)),
						HostURL: model.ToAPIString(""),
						Distro: model.DistroInfo{
							Id:       model.ToAPIString(""),
							Provider: model.ToAPIString(""),
						},
						StartedBy: model.ToAPIString(""),
						Type:      model.ToAPIString(""),
						User:      model.ToAPIString(""),
						Status:    model.ToAPIString(""),
					}
					expectedHosts = append(expectedHosts, nextModelHost)
				}
				expectedPages := &gimlet.ResponsePages{
					Next: &gimlet.Page{
						Key:             fmt.Sprintf("host%d", hostToStartAt+limit),
						Limit:           limit,
						Relation:        "next",
						BaseURL:         serviceContext.GetURL(),
						KeyQueryParam:   "host_id",
						LimitQueryParam: "limit",
					},
				}
				handler := &hostGetHandler{
					key:   cachedHosts[hostToStartAt].Id,
					limit: limit,
					sc:    &serviceContext,
				}

				validatePaginatedResponse(t, handler, expectedHosts, expectedPages)
			})
			Convey("then finding a key in the near the beginning of the set should produce"+
				" a full next and a limited previous page and a full set of models", func() {
				hostToStartAt := 50
				limit := 100
				expectedHosts := []model.Model{}
				for i := hostToStartAt; i < hostToStartAt+limit; i++ {
					nextModelHost := &model.APIHost{
						Id:      model.ToAPIString(fmt.Sprintf("host%d", i)),
						HostURL: model.ToAPIString(""),
						Distro: model.DistroInfo{
							Id:       model.ToAPIString(""),
							Provider: model.ToAPIString(""),
						},
						StartedBy: model.ToAPIString(""),
						Type:      model.ToAPIString(""),
						User:      model.ToAPIString(""),
						Status:    model.ToAPIString(""),
					}
					expectedHosts = append(expectedHosts, nextModelHost)
				}
				expectedPages := &gimlet.ResponsePages{
					Next: &gimlet.Page{
						Key:             fmt.Sprintf("host%d", hostToStartAt+limit),
						Limit:           limit,
						Relation:        "next",
						BaseURL:         serviceContext.GetURL(),
						KeyQueryParam:   "host_id",
						LimitQueryParam: "limit",
					},
				}
				handler := &hostGetHandler{
					sc:    &serviceContext,
					key:   cachedHosts[hostToStartAt].Id,
					limit: limit,
				}
				validatePaginatedResponse(t, handler, expectedHosts, expectedPages)
			})
			Convey("then finding the first key should produce only a next"+
				" page and a full set of models", func() {
				hostToStartAt := 0
				limit := 100
				expectedHosts := []model.Model{}
				for i := hostToStartAt; i < hostToStartAt+limit; i++ {
					nextModelHost := &model.APIHost{
						Id:      model.ToAPIString(fmt.Sprintf("host%d", i)),
						HostURL: model.ToAPIString(""),
						Distro: model.DistroInfo{
							Id:       model.ToAPIString(""),
							Provider: model.ToAPIString(""),
						},
						StartedBy: model.ToAPIString(""),
						Type:      model.ToAPIString(""),
						User:      model.ToAPIString(""),
						Status:    model.ToAPIString(""),
					}
					expectedHosts = append(expectedHosts, nextModelHost)
				}
				expectedPages := &gimlet.ResponsePages{
					Next: &gimlet.Page{
						Key:             fmt.Sprintf("host%d", hostToStartAt+limit),
						Limit:           limit,
						Relation:        "next",
						BaseURL:         serviceContext.GetURL(),
						KeyQueryParam:   "host_id",
						LimitQueryParam: "limit",
					},
				}
				handler := &hostGetHandler{
					sc:    &serviceContext,
					key:   cachedHosts[hostToStartAt].Id,
					limit: limit,
				}
				validatePaginatedResponse(t, handler, expectedHosts, expectedPages)
			})
		})
	})
}

func TestTasksByProjectAndCommitPaginator(t *testing.T) {
	numTasks := 300
	projectName := "project_1"
	commit := "commit_1"
	Convey("When paginating with a Connector", t, func() {
		serviceContext := data.MockConnector{}
		Convey("and there are tasks to be found", func() {
			cachedTasks := []task.Task{}
			for i := 0; i < numTasks; i++ {
				nextTask := task.Task{
					Id:       fmt.Sprintf("task_%d", i),
					Revision: commit,
					Project:  projectName,
				}
				cachedTasks = append(cachedTasks, nextTask)
			}
			serviceContext.MockTaskConnector.CachedTasks = cachedTasks
			Convey("then finding a key in the middle of the set should produce"+
				" a full next and previous page and a full set of models", func() {
				taskToStartAt := 100
				limit := 100
				expectedTasks := []model.Model{}
				for i := taskToStartAt; i < taskToStartAt+limit; i++ {
					serviceTask := &task.Task{
						Id:       fmt.Sprintf("task_%d", i),
						Revision: commit,
						Project:  projectName,
					}
					nextModelTask := &model.APITask{}
					err := nextModelTask.BuildFromService(serviceTask)
					So(err, ShouldBeNil)
					err = nextModelTask.BuildFromService(serviceContext.GetURL())
					So(err, ShouldBeNil)
					expectedTasks = append(expectedTasks, nextModelTask)
				}
				expectedPages := &PageResult{
					Next: &Page{
						Key:      fmt.Sprintf("task_%d", taskToStartAt+limit),
						Limit:    limit,
						Relation: "next",
					},
					Prev: &Page{
						Key:      fmt.Sprintf("task_%d", taskToStartAt-limit),
						Limit:    limit,
						Relation: "prev",
					},
				}
				tphArgs := tasksByProjectArgs{
					projectId:  projectName,
					commitHash: commit,
				}
				checkPaginatorResultMatches(tasksByProjectPaginator, fmt.Sprintf("task_%d", taskToStartAt),
					limit, &serviceContext, tphArgs, expectedPages, expectedTasks, nil)

			})
			Convey("then finding a key in the near the end of the set should produce"+
				" a limited next and full previous page and a full set of models", func() {
				taskToStartAt := 150
				limit := 100
				expectedTasks := []model.Model{}
				for i := taskToStartAt; i < taskToStartAt+limit; i++ {
					serviceTask := &task.Task{
						Id:       fmt.Sprintf("task_%d", i),
						Revision: commit,
						Project:  projectName,
					}
					nextModelTask := &model.APITask{}
					err := nextModelTask.BuildFromService(serviceTask)
					So(err, ShouldBeNil)
					err = nextModelTask.BuildFromService(serviceContext.GetURL())
					So(err, ShouldBeNil)
					expectedTasks = append(expectedTasks, nextModelTask)
				}
				expectedPages := &PageResult{
					Next: &Page{
						Key:      fmt.Sprintf("task_%d", taskToStartAt+limit),
						Limit:    50,
						Relation: "next",
					},
					Prev: &Page{
						Key:      fmt.Sprintf("task_%d", taskToStartAt-limit),
						Limit:    limit,
						Relation: "prev",
					},
				}
				tphArgs := tasksByProjectArgs{
					projectId:  projectName,
					commitHash: commit,
				}
				checkPaginatorResultMatches(tasksByProjectPaginator, fmt.Sprintf("task_%d", taskToStartAt),
					limit, &serviceContext, tphArgs, expectedPages, expectedTasks, nil)

			})
			Convey("then finding a key in the near the beginning of the set should produce"+
				" a full next and a limited previous page and a full set of models", func() {
				taskToStartAt := 50
				limit := 100
				expectedTasks := []model.Model{}
				for i := taskToStartAt; i < taskToStartAt+limit; i++ {
					serviceTask := &task.Task{
						Id:       fmt.Sprintf("task_%d", i),
						Revision: commit,
						Project:  projectName,
					}
					nextModelTask := &model.APITask{}
					err := nextModelTask.BuildFromService(serviceTask)
					So(err, ShouldBeNil)
					err = nextModelTask.BuildFromService(serviceContext.GetURL())
					So(err, ShouldBeNil)
					expectedTasks = append(expectedTasks, nextModelTask)
				}
				expectedPages := &PageResult{
					Next: &Page{
						Key:      fmt.Sprintf("task_%d", taskToStartAt+limit),
						Limit:    limit,
						Relation: "next",
					},
					Prev: &Page{
						Key:      fmt.Sprintf("task_%d", 0),
						Limit:    50,
						Relation: "prev",
					},
				}
				tphArgs := tasksByProjectArgs{
					projectId:  projectName,
					commitHash: commit,
				}
				checkPaginatorResultMatches(tasksByProjectPaginator, fmt.Sprintf("task_%d", taskToStartAt),
					limit, &serviceContext, tphArgs, expectedPages, expectedTasks, nil)

			})
			Convey("then finding a key in the last page should produce only a previous"+
				" page and a limited set of models", func() {
				taskToStartAt := 299
				limit := 100
				expectedTasks := []model.Model{}
				for i := taskToStartAt; i < numTasks; i++ {
					serviceTask := &task.Task{
						Id:       fmt.Sprintf("task_%d", i),
						Revision: commit,
						Project:  projectName,
					}
					nextModelTask := &model.APITask{}
					err := nextModelTask.BuildFromService(serviceTask)
					So(err, ShouldBeNil)
					err = nextModelTask.BuildFromService(serviceContext.GetURL())
					So(err, ShouldBeNil)
					expectedTasks = append(expectedTasks, nextModelTask)
				}
				expectedPages := &PageResult{
					Prev: &Page{
						Key:      fmt.Sprintf("task_%d", taskToStartAt-limit),
						Limit:    limit,
						Relation: "prev",
					},
				}
				tphArgs := tasksByProjectArgs{
					projectId:  projectName,
					commitHash: commit,
				}
				checkPaginatorResultMatches(tasksByProjectPaginator, fmt.Sprintf("task_%d", taskToStartAt),
					limit, &serviceContext, tphArgs, expectedPages, expectedTasks, nil)

			})
			Convey("then finding the first key should produce only a next"+
				" page and a full set of models", func() {
				taskToStartAt := 0
				limit := 100
				expectedTasks := []model.Model{}
				for i := taskToStartAt; i < taskToStartAt+limit; i++ {
					serviceTask := &task.Task{
						Id:       fmt.Sprintf("task_%d", i),
						Revision: commit,
						Project:  projectName,
					}
					nextModelTask := &model.APITask{}
					err := nextModelTask.BuildFromService(serviceTask)
					So(err, ShouldBeNil)
					err = nextModelTask.BuildFromService(serviceContext.GetURL())
					So(err, ShouldBeNil)
					expectedTasks = append(expectedTasks, nextModelTask)
				}
				expectedPages := &PageResult{
					Next: &Page{
						Key:      fmt.Sprintf("task_%d", taskToStartAt+limit),
						Limit:    limit,
						Relation: "next",
					},
				}
				tphArgs := tasksByProjectArgs{
					projectId:  projectName,
					commitHash: commit,
				}
				checkPaginatorResultMatches(tasksByProjectPaginator, fmt.Sprintf("task_%d", taskToStartAt),
					limit, &serviceContext, tphArgs, expectedPages, expectedTasks, nil)
			})
			Convey("when APIError is returned from DB, should percolate upward", func() {
				expectedErr := gimlet.ErrorResponse{
					StatusCode: http.StatusNotFound,
					Message:    "not found",
				}
				serviceContext.MockTaskConnector.StoredError = &expectedErr
				checkPaginatorResultMatches(tasksByProjectPaginator, "",
					0, &serviceContext, tasksByProjectArgs{}, nil, nil, &expectedErr)

			})
		})
	})
}

func TestTaskByBuildPaginator(t *testing.T) {
	numTasks := 300
	Convey("When paginating with a Connector", t, func() {
		serviceContext := data.MockConnector{
			URL: "http://evergreen.example.net",
		}
		Convey("and there are tasks to be found", func() {
			cachedTasks := []task.Task{}
			cachedOldTasks := []task.Task{}
			for i := 0; i < numTasks; i++ {
				nextTask := task.Task{
					Id: fmt.Sprintf("build%d", i),
				}
				cachedTasks = append(cachedTasks, nextTask)
			}
			for i := 0; i < 5; i++ {
				nextTask := task.Task{
					Id:        fmt.Sprintf("build0_%d", i),
					OldTaskId: "build0",
					Execution: i,
				}
				cachedOldTasks = append(cachedOldTasks, nextTask)
			}

			serviceContext.MockTaskConnector.CachedTasks = cachedTasks
			serviceContext.MockTaskConnector.CachedOldTasks = cachedOldTasks
			Convey("then finding a key in the middle of the set should produce"+
				" a full next and previous page and a full set of models", func() {
				taskToStartAt := 100
				limit := 100
				expectedTasks := []model.Model{}
				for i := taskToStartAt; i < taskToStartAt+limit; i++ {
					serviceModel := &task.Task{
						Id: fmt.Sprintf("build%d", i),
					}
					nextModelTask := &model.APITask{}
					err := nextModelTask.BuildFromService(serviceModel)
					So(err, ShouldBeNil)
					err = nextModelTask.BuildFromService(serviceContext.GetURL())
					So(err, ShouldBeNil)
					expectedTasks = append(expectedTasks, nextModelTask)
				}
				expectedPages := &gimlet.ResponsePages{
					Next: &gimlet.Page{
						Key:             fmt.Sprintf("build%d", taskToStartAt+limit),
						Limit:           limit,
						Relation:        "next",
						BaseURL:         serviceContext.GetURL(),
						KeyQueryParam:   "start_at",
						LimitQueryParam: "limit",
					},
				}

				tbh := &tasksByBuildHandler{
					limit: limit,
					key:   fmt.Sprintf("build%d", taskToStartAt),
					sc:    &serviceContext,
				}

				// SPARTA
				validatePaginatedResponse(t, tbh, expectedTasks, expectedPages)

			})
			Convey("then finding a key in the near the end of the set should produce"+
				" a limited next and full previous page and a full set of models", func() {
				taskToStartAt := 150
				limit := 100
				expectedTasks := []model.Model{}
				for i := taskToStartAt; i < taskToStartAt+limit; i++ {
					serviceModel := &task.Task{
						Id: fmt.Sprintf("build%d", i),
					}
					nextModelTask := &model.APITask{}
					err := nextModelTask.BuildFromService(serviceModel)
					So(err, ShouldBeNil)
					err = nextModelTask.BuildFromService(serviceContext.GetURL())
					So(err, ShouldBeNil)
					expectedTasks = append(expectedTasks, nextModelTask)
				}
				expectedPages := &gimlet.ResponsePages{
					Next: &gimlet.Page{
						Key:             fmt.Sprintf("build%d", taskToStartAt+limit),
						Limit:           limit,
						Relation:        "next",
						BaseURL:         serviceContext.GetURL(),
						KeyQueryParam:   "start_at",
						LimitQueryParam: "limit",
					},
				}

				tbh := &tasksByBuildHandler{
					limit: limit,
					key:   fmt.Sprintf("build%d", taskToStartAt),
					sc:    &serviceContext,
				}

				validatePaginatedResponse(t, tbh, expectedTasks, expectedPages)

			})
			Convey("then finding a key in the near the beginning of the set should produce"+
				" a full next and a limited previous page and a full set of models", func() {
				taskToStartAt := 50
				limit := 100
				expectedTasks := []model.Model{}
				for i := taskToStartAt; i < taskToStartAt+limit; i++ {
					serviceModel := &task.Task{
						Id: fmt.Sprintf("build%d", i),
					}
					nextModelTask := &model.APITask{}
					err := nextModelTask.BuildFromService(serviceModel)
					So(err, ShouldBeNil)
					err = nextModelTask.BuildFromService(serviceContext.GetURL())
					So(err, ShouldBeNil)
					expectedTasks = append(expectedTasks, nextModelTask)
				}
				expectedPages := &gimlet.ResponsePages{
					Next: &gimlet.Page{
						Key:             fmt.Sprintf("build%d", taskToStartAt+limit),
						Limit:           limit,
						Relation:        "next",
						BaseURL:         serviceContext.GetURL(),
						KeyQueryParam:   "start_at",
						LimitQueryParam: "limit",
					},
				}

				tbh := &tasksByBuildHandler{
					limit: limit,
					key:   fmt.Sprintf("build%d", taskToStartAt),
					sc:    &serviceContext,
				}

				validatePaginatedResponse(t, tbh, expectedTasks, expectedPages)
			})

			Convey("then finding the first key should produce only a next"+
				" page and a full set of models", func() {
				taskToStartAt := 0
				limit := 100
				expectedTasks := []model.Model{}
				for i := taskToStartAt; i < taskToStartAt+limit; i++ {
					serviceModel := &task.Task{
						Id: fmt.Sprintf("build%d", i),
					}
					nextModelTask := &model.APITask{}
					err := nextModelTask.BuildFromService(serviceModel)
					So(err, ShouldBeNil)
					err = nextModelTask.BuildFromService(serviceContext.GetURL())
					So(err, ShouldBeNil)
					expectedTasks = append(expectedTasks, nextModelTask)
				}
				expectedPages := &gimlet.ResponsePages{
					Next: &gimlet.Page{
						Key:             fmt.Sprintf("build%d", taskToStartAt+limit),
						Limit:           limit,
						Relation:        "next",
						BaseURL:         serviceContext.GetURL(),
						KeyQueryParam:   "start_at",
						LimitQueryParam: "limit",
					},
				}

				tbh := &tasksByBuildHandler{
					limit: limit,
					key:   fmt.Sprintf("build%d", taskToStartAt),
					sc:    &serviceContext,
				}

				validatePaginatedResponse(t, tbh, expectedTasks, expectedPages)
			})

			Convey("pagination with tasks with previous executions", func() {
				expectedTasks := []model.Model{}
				serviceModel := &task.Task{
					Id: "build0",
				}
				nextModelTask := &model.APITask{}
				err := nextModelTask.BuildFromService(serviceModel)
				So(err, ShouldBeNil)
				err = nextModelTask.BuildPreviousExecutions(cachedOldTasks)
				So(err, ShouldBeNil)
				err = nextModelTask.BuildFromService(serviceContext.GetURL())
				So(err, ShouldBeNil)
				expectedTasks = append(expectedTasks, nextModelTask)
				expectedPages := &gimlet.ResponsePages{
					Next: &gimlet.Page{
						Key:             "build1",
						Limit:           1,
						Relation:        "next",
						BaseURL:         serviceContext.GetURL(),
						KeyQueryParam:   "start_at",
						LimitQueryParam: "limit",
					},
				}

				tbh := &tasksByBuildHandler{
					limit:              1,
					key:                "build0",
					sc:                 &serviceContext,
					fetchAllExecutions: true,
				}

				validatePaginatedResponse(t, tbh, expectedTasks, expectedPages)
			})
		})
	})
}

func TestTestPaginator(t *testing.T) {
	numTests := 300
	Convey("When paginating with a Connector", t, func() {
		serviceContext := data.MockConnector{}
		Convey("and there are tasks with tests to be found", func() {
			cachedTests := []testresult.TestResult{}
			for i := 0; i < numTests; i++ {
				status := "pass"
				if i%2 == 0 {
					status = "fail"
				}
				nextTest := testresult.TestResult{
					ID:     bson.ObjectId(fmt.Sprintf("object_id_%d_", i)),
					Status: status,
				}
				cachedTests = append(cachedTests, nextTest)
			}
			serviceContext.MockTestConnector.CachedTests = cachedTests
			Convey("then finding a key in the middle of the set should produce"+
				" a full next and previous page and a full set of models", func() {
				testToStartAt := 100
				limit := 100
				expectedTests := []model.Model{}
				for i := testToStartAt; i < testToStartAt+limit; i++ {
					status := "pass"
					if i%2 == 0 {
						status = "fail"
					}
					nextModelTest := &model.APITest{
						StartTime: model.NewTime(time.Unix(0, 0)),
						EndTime:   model.NewTime(time.Unix(0, 0)),
						Status:    model.ToAPIString(status),
					}
					expectedTests = append(expectedTests, nextModelTest)
				}
				expectedPages := &PageResult{
					Next: &Page{
						Key:      fmt.Sprintf("object_id_%d_", testToStartAt+limit),
						Limit:    limit,
						Relation: "next",
					},
					Prev: &Page{
						Key:      fmt.Sprintf("object_id_%d_", testToStartAt-limit),
						Limit:    limit,
						Relation: "prev",
					},
				}
				args := testGetHandlerArgs{}
				checkPaginatorResultMatches(testPaginator, fmt.Sprintf("object_id_%d_", testToStartAt),
					limit, &serviceContext, args, expectedPages, expectedTests, nil)

			})
			Convey("then finding a key in the near the end of the set should produce"+
				" a limited next and full previous page and a full set of models", func() {
				testToStartAt := 150
				limit := 100
				expectedTests := []model.Model{}
				for i := testToStartAt; i < testToStartAt+limit; i++ {
					status := "pass"
					if i%2 == 0 {
						status = "fail"
					}
					nextModelTest := &model.APITest{
						StartTime: model.NewTime(time.Unix(0, 0)),
						EndTime:   model.NewTime(time.Unix(0, 0)),
						Status:    model.ToAPIString(status),
					}
					expectedTests = append(expectedTests, nextModelTest)
				}
				expectedPages := &PageResult{
					Next: &Page{
						Key:      fmt.Sprintf("object_id_%d_", testToStartAt+limit),
						Limit:    50,
						Relation: "next",
					},
					Prev: &Page{
						Key:      fmt.Sprintf("object_id_%d_", testToStartAt-limit),
						Limit:    limit,
						Relation: "prev",
					},
				}
				args := testGetHandlerArgs{}
				checkPaginatorResultMatches(testPaginator, fmt.Sprintf("object_id_%d_", testToStartAt),
					limit, &serviceContext, args, expectedPages, expectedTests, nil)

			})
			Convey("then finding a key in the near the beginning of the set should produce"+
				" a full next and a limited previous page and a full set of models", func() {
				testToStartAt := 50
				limit := 100
				expectedTests := []model.Model{}
				for i := testToStartAt; i < testToStartAt+limit; i++ {
					status := "pass"
					if i%2 == 0 {
						status = "fail"
					}
					nextModelTest := &model.APITest{
						StartTime: model.NewTime(time.Unix(0, 0)),
						EndTime:   model.NewTime(time.Unix(0, 0)),
						Status:    model.ToAPIString(status),
					}
					expectedTests = append(expectedTests, nextModelTest)
				}
				expectedPages := &PageResult{
					Next: &Page{
						Key:      fmt.Sprintf("object_id_%d_", testToStartAt+limit),
						Limit:    limit,
						Relation: "next",
					},
					Prev: &Page{
						Key:      fmt.Sprintf("object_id_%d_", 0),
						Limit:    50,
						Relation: "prev",
					},
				}
				args := testGetHandlerArgs{}
				checkPaginatorResultMatches(testPaginator, fmt.Sprintf("object_id_%d_", testToStartAt),
					limit, &serviceContext, args, expectedPages, expectedTests, nil)

			})
			Convey("then finding a key in the last page should produce only a previous"+
				" page and a limited set of models", func() {
				testToStartAt := 299
				limit := 100
				expectedTests := []model.Model{}
				for i := testToStartAt; i < numTests; i++ {
					status := "pass"
					if i%2 == 0 {
						status = "fail"
					}
					nextModelTest := &model.APITest{
						StartTime: model.NewTime(time.Unix(0, 0)),
						EndTime:   model.NewTime(time.Unix(0, 0)),
						Status:    model.ToAPIString(status),
					}
					expectedTests = append(expectedTests, nextModelTest)
				}
				expectedPages := &PageResult{
					Prev: &Page{
						Key:      fmt.Sprintf("object_id_%d_", testToStartAt-limit),
						Limit:    limit,
						Relation: "prev",
					},
				}
				args := testGetHandlerArgs{}
				checkPaginatorResultMatches(testPaginator, fmt.Sprintf("object_id_%d_", testToStartAt),
					limit, &serviceContext, args, expectedPages, expectedTests, nil)

			})
			Convey("then finding the first key should produce only a next"+
				" page and a full set of models", func() {
				testToStartAt := 0
				limit := 100
				expectedTests := []model.Model{}
				for i := testToStartAt; i < testToStartAt+limit; i++ {
					status := "pass"
					if i%2 == 0 {
						status = "fail"
					}
					nextModelTest := &model.APITest{
						StartTime: model.NewTime(time.Unix(0, 0)),
						EndTime:   model.NewTime(time.Unix(0, 0)),
						Status:    model.ToAPIString(status),
					}
					expectedTests = append(expectedTests, nextModelTest)
				}
				expectedPages := &PageResult{
					Next: &Page{
						Key:      fmt.Sprintf("object_id_%d_", testToStartAt+limit),
						Limit:    limit,
						Relation: "next",
					},
				}
				args := testGetHandlerArgs{}
				checkPaginatorResultMatches(testPaginator, fmt.Sprintf("object_id_%d_", testToStartAt),
					limit, &serviceContext, args, expectedPages, expectedTests, nil)
			})
		})
	})
}

func TestTaskExecutionPatchPrepare(t *testing.T) {
	Convey("With handler and a project context and user", t, func() {
		tep := &TaskExecutionPatchHandler{}

		projCtx := serviceModel.Context{
			Task: &task.Task{
				Id:        "testTaskId",
				Priority:  0,
				Activated: false,
			},
		}
		u := user.DBUser{
			Id: "testUser",
		}
		ctx := context.Background()
		Convey("then should error on empty body", func() {
			req, err := http.NewRequest("PATCH", "task/testTaskId", &bytes.Buffer{})
			So(err, ShouldBeNil)
			ctx = gimlet.AttachUser(ctx, &u)
			ctx = context.WithValue(ctx, RequestContext, &projCtx)
			err = tep.ParseAndValidate(ctx, req)
			So(err, ShouldNotBeNil)
			expectedErr := gimlet.ErrorResponse{
				Message:    "No request body sent",
				StatusCode: http.StatusBadRequest,
			}
			So(err, ShouldResemble, expectedErr)
		})
		Convey("then should error on body with wrong type", func() {
			str := "nope"
			badBod := &struct {
				Activated *string
			}{
				Activated: &str,
			}
			res, err := json.Marshal(badBod)
			So(err, ShouldBeNil)
			buf := bytes.NewBuffer(res)

			req, err := http.NewRequest("PATCH", "task/testTaskId", buf)
			So(err, ShouldBeNil)
			ctx = gimlet.AttachUser(ctx, &u)
			ctx = context.WithValue(ctx, RequestContext, &projCtx)
			err = tep.ParseAndValidate(ctx, req)
			So(err, ShouldNotBeNil)
			expectedErr := gimlet.ErrorResponse{
				Message: fmt.Sprintf("Incorrect type given, expecting '%s' "+
					"but receieved '%s'",
					"bool", "string"),
				StatusCode: http.StatusBadRequest,
			}
			So(err, ShouldResemble, expectedErr)
		})
		Convey("then should error when fields not set", func() {
			badBod := &struct {
				Activated *string
			}{}
			res, err := json.Marshal(badBod)
			So(err, ShouldBeNil)
			buf := bytes.NewBuffer(res)

			req, err := http.NewRequest("PATCH", "task/testTaskId", buf)
			So(err, ShouldBeNil)
			ctx = gimlet.AttachUser(ctx, &u)
			ctx = context.WithValue(ctx, RequestContext, &projCtx)
			err = tep.ParseAndValidate(ctx, req)
			So(err, ShouldNotBeNil)
			expectedErr := gimlet.ErrorResponse{
				Message:    "Must set 'activated' or 'priority'",
				StatusCode: http.StatusBadRequest,
			}
			So(err, ShouldResemble, expectedErr)
		})
		Convey("then should set it's Activated and Priority field when set", func() {
			goodBod := &struct {
				Activated bool
				Priority  int
			}{
				Activated: true,
				Priority:  100,
			}
			res, err := json.Marshal(goodBod)
			So(err, ShouldBeNil)
			buf := bytes.NewBuffer(res)

			req, err := http.NewRequest("PATCH", "task/testTaskId", buf)
			So(err, ShouldBeNil)
			ctx = gimlet.AttachUser(ctx, &u)
			ctx = context.WithValue(ctx, RequestContext, &projCtx)
			err = tep.ParseAndValidate(ctx, req)
			So(err, ShouldBeNil)
			So(*tep.Activated, ShouldBeTrue)
			So(*tep.Priority, ShouldEqual, 100)

			Convey("and task and user should be set", func() {
				So(tep.task, ShouldNotBeNil)
				So(tep.task.Id, ShouldEqual, "testTaskId")
				So(tep.user.Username(), ShouldEqual, "testUser")
			})
		})
	})
}

func TestTaskExecutionPatchExecute(t *testing.T) {
	Convey("With a task in the DB and a Connector", t, func() {
		sc := data.MockConnector{}
		testTask := task.Task{
			Id:        "testTaskId",
			Activated: false,
			Priority:  10,
		}
		sc.MockTaskConnector.CachedTasks = append(sc.MockTaskConnector.CachedTasks, testTask)
		ctx := context.Background()
		Convey("then setting priority should change it's priority", func() {
			act := true
			var prio int64 = 100

			tep := &TaskExecutionPatchHandler{
				Activated: &act,
				Priority:  &prio,
				task: &task.Task{
					Id: "testTaskId",
				},
				user: &user.DBUser{
					Id: "testUser",
				},
			}
			res, err := tep.Execute(ctx, &sc)
			So(err, ShouldBeNil)
			So(len(res.Result), ShouldEqual, 1)
			resModel := res.Result[0]
			resTask, ok := resModel.(*model.APITask)
			So(ok, ShouldBeTrue)
			So(resTask.Priority, ShouldEqual, int64(100))
			So(resTask.Activated, ShouldBeTrue)
			So(model.FromAPIString(resTask.ActivatedBy), ShouldEqual, "testUser")
		})
	})
}

func TestTaskResetPrepare(t *testing.T) {
	Convey("With handler and a project context and user", t, func() {
		trh := &taskRestartHandler{}

		projCtx := serviceModel.Context{
			Task: &task.Task{
				Id:        "testTaskId",
				Priority:  0,
				Activated: false,
			},
		}
		u := user.DBUser{
			Id: "testUser",
		}
		ctx := context.Background()

		Convey("should error on empty project", func() {
			req, err := http.NewRequest("POST", "task/testTaskId/restart", &bytes.Buffer{})
			So(err, ShouldBeNil)
			ctx = gimlet.AttachUser(ctx, &u)
			ctx = context.WithValue(ctx, RequestContext, &projCtx)
			err = trh.ParseAndValidate(ctx, req)
			So(err, ShouldNotBeNil)
			expectedErr := "Project not found"
			So(err.Error(), ShouldContainSubstring, expectedErr)
		})
		Convey("then should error on empty task", func() {
			projCtx.Task = nil
			req, err := http.NewRequest("POST", "task/testTaskId/restart", &bytes.Buffer{})
			So(err, ShouldBeNil)
			ctx = gimlet.AttachUser(ctx, &u)
			ctx = context.WithValue(ctx, RequestContext, &projCtx)
			err = trh.ParseAndValidate(ctx, req)
			So(err, ShouldNotBeNil)
			expectedErr := gimlet.ErrorResponse{
				Message:    "Task not found",
				StatusCode: http.StatusNotFound,
			}

			So(err, ShouldResemble, expectedErr)
		})
	})
}

func TestTaskGetHandler(t *testing.T) {
	Convey("With test server with a handler and mock data", t, func() {
		rm := getTaskRouteManager("/tasks/{task_id}", 2)
		sc := &data.MockConnector{}
		sc.SetPrefix("rest")

		Convey("and task is in the service context", func() {
			sc.MockTaskConnector.CachedTasks = []task.Task{
				{Id: "testTaskId", Project: "testProject"},
			}
			sc.MockTaskConnector.CachedOldTasks = []task.Task{
				{Id: "testTaskId_0",
					OldTaskId: "testTaskId",
				},
			}

			app := gimlet.NewApp()
			app.SetPrefix(sc.GetPrefix())
			rm.Register(app, sc)
			So(app.Resolve(), ShouldBeNil)
			r, err := app.Router()
			So(err, ShouldBeNil)

			Convey("a request with a user should then return no error and a task should"+
				" should be returned", func() {
				req, err := http.NewRequest("GET", "/rest/v2/tasks/testTaskId", nil)
				So(err, ShouldBeNil)
				req.Header.Add("Api-Key", "Key")
				req.Header.Add("Api-User", "User")

				sc.MockUserConnector.CachedUsers = map[string]*user.DBUser{
					"User": &user.DBUser{
						APIKey: "Key",
						Id:     "User",
					},
				}

				req = req.WithContext(gimlet.AttachUser(req.Context(), sc.MockUserConnector.CachedUsers["User"]))

				rr := httptest.NewRecorder()
				r.ServeHTTP(rr, req)
				So(rr.Code, ShouldEqual, http.StatusOK)

				res := model.APITask{}
				err = json.Unmarshal(rr.Body.Bytes(), &res)
				So(err, ShouldBeNil)
				So(model.FromAPIString(res.Id), ShouldEqual, "testTaskId")
				So(model.FromAPIString(res.ProjectId), ShouldEqual, "testProject")
				So(len(res.PreviousExecutions), ShouldEqual, 0)
			})
			Convey("a request without a user should then return a 404 error and a task should"+
				" should be not returned", func() {
				req, err := http.NewRequest("GET", "/rest/v2/tasks/testTaskId", nil)
				So(err, ShouldBeNil)
				req.Header.Add("Api-Key", "Key")
				req.Header.Add("Api-User", "User")

				rr := httptest.NewRecorder()
				r.ServeHTTP(rr, req)
				So(rr.Code, ShouldEqual, http.StatusNotFound)
			})
			Convey("and old tasks are available", func() {
				sc.MockTaskConnector.CachedTasks[0].Execution = 1
				sc.MockUserConnector.CachedUsers = map[string]*user.DBUser{
					"User": &user.DBUser{
						APIKey: "Key",
						Id:     "User",
					},
				}

				Convey("a test that requests old executions should receive them", func() {
					req, err := http.NewRequest("GET", "/rest/v2/tasks/testTaskId?fetch_all_executions=", nil)
					So(err, ShouldBeNil)
					req.Header.Add("Api-Key", "Key")
					req.Header.Add("Api-User", "User")
					req = req.WithContext(gimlet.AttachUser(req.Context(), sc.MockUserConnector.CachedUsers["User"]))

					rr := httptest.NewRecorder()
					r.ServeHTTP(rr, req)
					So(rr.Code, ShouldEqual, http.StatusOK)

					res := model.APITask{}
					err = json.Unmarshal(rr.Body.Bytes(), &res)
					So(err, ShouldBeNil)
					So(len(res.PreviousExecutions), ShouldEqual, 1)
				})
				Convey("a test that doesn't request old executions should not receive them", func() {
					req, err := http.NewRequest("GET", "/rest/v2/tasks/testTaskId", nil)
					So(err, ShouldBeNil)
					req.Header.Add("Api-Key", "Key")
					req.Header.Add("Api-User", "User")
					req = req.WithContext(gimlet.AttachUser(req.Context(), sc.MockUserConnector.CachedUsers["User"]))

					rr := httptest.NewRecorder()
					r.ServeHTTP(rr, req)
					So(rr.Code, ShouldEqual, http.StatusOK)

					res := model.APITask{}
					err = json.Unmarshal(rr.Body.Bytes(), &res)
					So(err, ShouldBeNil)
					So(len(res.PreviousExecutions), ShouldEqual, 0)
				})

			})
		})
	})
}

func TestTaskResetExecute(t *testing.T) {
	Convey("With a task returned by the Connector", t, func() {
		sc := data.MockConnector{}
		timeNow := time.Now()
		testTask := task.Task{
			Id:           "testTaskId",
			Activated:    false,
			Secret:       "initialSecret",
			DispatchTime: timeNow,
		}
		sc.MockTaskConnector.CachedTasks = append(sc.MockTaskConnector.CachedTasks, testTask)
		ctx := context.Background()
		Convey("and an error from the service function", func() {
			sc.MockTaskConnector.StoredError = fmt.Errorf("could not reset task")

			trh := &taskRestartHandler{
				taskId:   "testTaskId",
				username: "testUser",
			}

			_, err := trh.Execute(ctx, &sc)
			So(err, ShouldNotBeNil)
			apiErr, ok := err.(gimlet.ErrorResponse)
			So(ok, ShouldBeTrue)
			So(apiErr.StatusCode, ShouldEqual, http.StatusBadRequest)

		})

		Convey("calling TryReset should reset the task", func() {

			trh := &taskRestartHandler{
				taskId:   "testTaskId",
				username: "testUser",
			}

			res, err := trh.Execute(ctx, &sc)
			So(err, ShouldBeNil)
			So(len(res.Result), ShouldEqual, 1)
			resModel := res.Result[0]
			resTask, ok := resModel.(*model.APITask)
			So(ok, ShouldBeTrue)
			So(resTask.Activated, ShouldBeTrue)
			So(resTask.DispatchTime, ShouldResemble, model.APIZeroTime)
			dbTask, err := sc.FindTaskById("testTaskId")
			So(err, ShouldBeNil)
			So(string(dbTask.Secret), ShouldNotResemble, "initialSecret")
		})
	})

}

func checkPaginatorResultMatches(paginator PaginatorFunc, key string, limit int,
	sc data.Connector, args interface{}, expectedPages *PageResult,
	expectedModels []model.Model, expectedErr error) {

	res, pages, err := paginator(key, limit, args, sc)
	So(errors.Cause(err), ShouldResemble, expectedErr)
	So(len(res), ShouldEqual, len(expectedModels))
	for ix := range expectedModels {
		dbModel, err := res[ix].ToService()
		So(err, ShouldBeNil)
		expectedModel, err := expectedModels[ix].ToService()
		So(err, ShouldBeNil)
		So(dbModel, ShouldResemble, expectedModel)
	}
	So(pages, ShouldResemble, expectedPages)
}

func validatePaginatedResponse(t *testing.T, h gimlet.RouteHandler, expected []model.Model, pages *gimlet.ResponsePages) {
	if !assert.NotNil(t, h) {
		return
	}
	if !assert.NotNil(t, pages) {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resp := h.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())

	rpg := resp.Pages()
	if !assert.NotNil(t, rpg) {
		return
	}

	assert.True(t, pages.Next != nil || pages.Prev != nil)
	assert.True(t, rpg.Next != nil || rpg.Prev != nil)
	if pages.Next != nil {
		assert.Equal(t, pages.Next.Key, rpg.Next.Key)
		assert.Equal(t, pages.Next.Limit, rpg.Next.Limit)
		assert.Equal(t, pages.Next.Relation, rpg.Next.Relation)
	} else if pages.Prev != nil {
		assert.Equal(t, pages.Prev.Key, rpg.Prev.Key)
		assert.Equal(t, pages.Prev.Limit, rpg.Prev.Limit)
		assert.Equal(t, pages.Prev.Relation, rpg.Prev.Relation)
	}

	assert.EqualValues(t, pages, rpg)

	data, ok := resp.Data().([]interface{})
	assert.True(t, ok)

	if !assert.Equal(t, len(expected), len(data)) {
		return
	}

	for idx := range expected {
		m, ok := data[idx].(model.Model)

		if assert.True(t, ok) {
			assert.Equal(t, expected[idx], m)
		}
	}
}
