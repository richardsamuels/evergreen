package route

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

////////////////////////////////////////////////////////////////////////
//
// PATCH /rest/v2/patches/{patch_id}

type patchChangeStatusHandler struct {
	Activated *bool  `json:"activated"`
	Priority  *int64 `json:"priority"`

	patchId string
	sc      data.Connector
}

func makeChangePatchStatus(sc data.Connector) gimlet.RouteHandler {
	return &patchChangeStatusHandler{
		sc: sc,
	}
}

func (p *patchChangeStatusHandler) Factory() gimlet.RouteHandler {
	return &patchChangeStatusHandler{
		sc: p.sc,
	}
}

func (p *patchChangeStatusHandler) Parse(ctx context.Context, r *http.Request) error {
	p.patchId = gimlet.GetVars(r)["patch_id"]
	body := util.NewRequestReader(r)
	defer body.Close()

	if err := util.ReadJSONInto(body, p); err != nil {
		return errors.Wrap(err, "Argument read error")
	}

	if p.Activated == nil && p.Priority == nil {
		return gimlet.ErrorResponse{
			Message:    "Must set 'activated' or 'priority'",
			StatusCode: http.StatusBadRequest,
		}
	}
	return nil
}

func (p *patchChangeStatusHandler) Run(ctx context.Context) gimlet.Responder {
	user := MustHaveUser(ctx)

	if p.Priority != nil {
		priority := *p.Priority
		if ok := validPriority(priority, user, p.sc); !ok {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				Message: fmt.Sprintf("Insufficient privilege to set priority to %d, "+
					"non-superusers can only set priority at or below %d", priority, evergreen.MaxTaskPriority),
				StatusCode: http.StatusForbidden,
			})
		}
		if err := p.sc.SetPatchPriority(p.patchId, priority); err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
		}
	}
	if p.Activated != nil {
		if err := p.sc.SetPatchActivated(p.patchId, user.Username(), *p.Activated); err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
		}
	}
	foundPatch, err := p.sc.FindPatchById(p.patchId)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
	}

	patchModel := &model.APIPatch{}
	if err = patchModel.BuildFromService(*foundPatch); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
	}
	return gimlet.NewJSONResponse(patchModel)
}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/patches/{patch_id}

type patchByIdHandler struct {
	patchId string
	sc      data.Connector
}

func makeFetchPatchByID(sc data.Connector) gimlet.RouteHandler {
	return &patchByIdHandler{
		sc: sc,
	}
}

func (p *patchByIdHandler) Factory() gimlet.RouteHandler {
	return &patchByIdHandler{sc: p.sc}
}

func (p *patchByIdHandler) Parse(ctx context.Context, r *http.Request) error {
	p.patchId = gimlet.GetVars(r)["patch_id"]
	return nil
}

func (p *patchByIdHandler) Run(ctx context.Context) gimlet.Responder {
	foundPatch, err := p.sc.FindPatchById(p.patchId)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
	}

	patchModel := &model.APIPatch{}
	err = patchModel.BuildFromService(*foundPatch)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "API model error"))
	}

	return gimlet.NewJSONResponse(patchModel)
}

////////////////////////////////////////////////////////////////////////
//
// Handler for fetching current users patches
//
//    /patches/mine

type patchesByUserHandler struct {
	PaginationExecutor
}

type patchesByUserArgs struct {
	user string
}

func getPatchesByUserManager(route string, version int) *RouteManager {
	p := &patchesByUserHandler{}
	return &RouteManager{
		Route:   route,
		Version: version,
		Methods: []MethodHandler{
			{
				MethodType:     http.MethodGet,
				Authenticator:  &RequireUserAuthenticator{},
				RequestHandler: p.Handler(),
			},
		},
	}
}

func (p *patchesByUserHandler) Handler() RequestHandler {
	return &patchesByUserHandler{PaginationExecutor{
		KeyQueryParam:   "start_at",
		LimitQueryParam: "limit",
		Paginator:       patchesByUserPaginator,
		Args:            patchesByUserArgs{},
	}}
}

func (p *patchesByUserHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	p.Args = patchesByUserArgs{gimlet.GetVars(r)["user_id"]}

	return p.PaginationExecutor.ParseAndValidate(ctx, r)
}

func patchesByUserPaginator(key string, limit int, args interface{}, sc data.Connector) ([]model.Model, *PageResult, error) {
	user := args.(patchesByUserArgs).user
	grip.Debugln("getting : ", limit, "patches for user: ", user, " starting from time: ", key)
	var ts time.Time
	var err error
	if key == "" {
		ts = time.Now()
	} else {
		ts, err = time.ParseInLocation(model.APITimeFormat, key, time.UTC)
		if err != nil {
			return []model.Model{}, nil, gimlet.ErrorResponse{
				Message:    fmt.Sprintf("problem parsing time from '%s' (%s)", key, err.Error()),
				StatusCode: http.StatusBadRequest,
			}
		}
	}
	// sortAsc set to false in order to display patches in desc chronological order
	patches, err := sc.FindPatchesByUser(user, ts, limit*2, false)
	if err != nil {
		return []model.Model{}, nil, errors.Wrap(err, "Database error")
	}
	if len(patches) <= 0 {
		return []model.Model{}, nil, gimlet.ErrorResponse{
			Message:    "no patches found",
			StatusCode: http.StatusNotFound,
		}
	}

	// Make the previous page
	prevPatches, err := sc.FindPatchesByUser(user, ts, limit, true)
	if err != nil {
		return []model.Model{}, nil, errors.Wrap(err, "Database error")
	}
	// populate the page info structure
	pages := &PageResult{}
	if len(patches) > limit {
		pages.Next = &Page{
			Relation: "next",
			Key:      model.NewTime(patches[limit].CreateTime).String(),
			Limit:    len(patches) - limit,
		}
	}
	if len(prevPatches) >= 1 {
		pages.Prev = &Page{
			Relation: "prev",
			Key:      model.NewTime(prevPatches[len(prevPatches)-1].CreateTime).String(),
			Limit:    len(prevPatches),
		}
	}

	// truncate results data if there's a next page.
	if pages.Next != nil {
		patches = patches[:limit]
	}
	models := []model.Model{}
	for _, info := range patches {
		patchModel := &model.APIPatch{}
		if err = patchModel.BuildFromService(info); err != nil {
			return []model.Model{}, nil, gimlet.ErrorResponse{
				Message:    "problem converting patch document",
				StatusCode: http.StatusInternalServerError,
			}
		}

		models = append(models, patchModel)
	}

	return models, pages, nil
}

////////////////////////////////////////////////////////////////////////
//
// Handler for the patches for a project
//
//    /projects/{project_id}/patches

type patchesByProjectArgs struct {
	projectId string
}

func getPatchesByProjectManager(route string, version int) *RouteManager {
	p := &patchesByProjectHandler{}
	return &RouteManager{
		Route:   route,
		Version: version,
		Methods: []MethodHandler{
			{
				MethodType:     http.MethodGet,
				Authenticator:  &NoAuthAuthenticator{},
				RequestHandler: p.Handler(),
			},
		},
	}
}

type patchesByProjectHandler struct {
	PaginationExecutor
}

func (p *patchesByProjectHandler) Handler() RequestHandler {
	return &patchesByProjectHandler{PaginationExecutor{
		KeyQueryParam:   "start_at",
		LimitQueryParam: "limit",
		Paginator:       patchesByProjectPaginator,
		Args:            patchesByProjectArgs{},
	}}
}

func (p *patchesByProjectHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	p.Args = patchesByProjectArgs{projectId: gimlet.GetVars(r)["project_id"]}

	return p.PaginationExecutor.ParseAndValidate(ctx, r)
}

func patchesByProjectPaginator(key string, limit int, args interface{}, sc data.Connector) ([]model.Model, *PageResult, error) {
	proj := args.(patchesByProjectArgs).projectId
	grip.Debugln("getting patches for project: ", proj, " starting from time: ", key)
	var ts time.Time
	var err error
	if key == "" {
		ts = time.Now()
	} else {
		ts, err = time.ParseInLocation(model.APITimeFormat, key, time.UTC)
		if err != nil {
			return []model.Model{}, nil, gimlet.ErrorResponse{
				Message:    fmt.Sprintf("problem parsing time from '%s' (%s)", key, err.Error()),
				StatusCode: http.StatusBadRequest,
			}
		}
	}
	// sortDir is set to -1 in order to display patches in reverse chronological order
	patches, err := sc.FindPatchesByProject(proj, ts, limit*2, false)
	if err != nil {
		return []model.Model{}, nil, errors.Wrap(err, "Database error")
	}
	if len(patches) <= 0 {
		return []model.Model{}, nil, gimlet.ErrorResponse{
			Message:    "no patches found",
			StatusCode: http.StatusNotFound,
		}
	}

	// Make the previous page
	prevPatches, err := sc.FindPatchesByProject(proj, ts, limit, true)
	if err != nil {
		return []model.Model{}, nil, errors.Wrap(err, "Database error")
	}
	// populate the page info structure
	pages := &PageResult{}
	if len(patches) > limit {
		pages.Next = &Page{
			Relation: "next",
			Key:      model.NewTime(patches[limit].CreateTime).String(),
			Limit:    len(patches) - limit,
		}
	}
	if len(prevPatches) >= 1 {
		pages.Prev = &Page{
			Relation: "prev",
			Key:      model.NewTime(prevPatches[len(prevPatches)-1].CreateTime).String(),
			Limit:    len(prevPatches),
		}
	}

	// truncate results data if there's a next page.
	if pages.Next != nil {
		patches = patches[:limit]
	}
	models := []model.Model{}
	for _, info := range patches {
		patchModel := &model.APIPatch{}
		if err = patchModel.BuildFromService(info); err != nil {
			return []model.Model{}, nil, gimlet.ErrorResponse{
				Message:    "problem converting patch document",
				StatusCode: http.StatusInternalServerError,
			}
		}

		models = append(models, patchModel)
	}

	return models, pages, nil
}

////////////////////////////////////////////////////////////////////////
//
// Handler for aborting patches by id
//
//    /patches/{patch_id}/abort

func getPatchAbortManager(route string, version int) *RouteManager {
	p := &patchAbortHandler{}
	return &RouteManager{
		Route:   route,
		Version: version,
		Methods: []MethodHandler{
			{
				MethodType:     http.MethodPost,
				Authenticator:  &RequireUserAuthenticator{},
				RequestHandler: p.Handler(),
			},
		},
	}
}

type patchAbortHandler struct {
	patchId string
}

func (p *patchAbortHandler) Handler() RequestHandler {
	return &patchAbortHandler{}
}

func (p *patchAbortHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	p.patchId = gimlet.GetVars(r)["patch_id"]
	return nil
}

func (p *patchAbortHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	usr := MustHaveUser(ctx)
	err := sc.AbortPatch(p.patchId, usr.Id)
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "Abort error")
	}

	// Patch may be deleted by abort (eg not finalized) and not found here
	foundPatch, err := sc.FindPatchById(p.patchId)
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "Database error")
	}
	patchModel := &model.APIPatch{}
	err = patchModel.BuildFromService(*foundPatch)

	if err != nil {
		return ResponseData{}, errors.Wrap(err, "API model error")
	}

	return ResponseData{
		Result: []model.Model{patchModel},
	}, nil
}

////////////////////////////////////////////////////////////////////////
//
// Handler for restarting patches by id
//
//    /patches/{patch_id}/restart

func getPatchRestartManager(route string, version int) *RouteManager {
	p := &patchRestartHandler{}
	return &RouteManager{
		Route:   route,
		Version: version,
		Methods: []MethodHandler{
			{
				MethodType:     http.MethodPost,
				Authenticator:  &RequireUserAuthenticator{},
				RequestHandler: p.Handler(),
			},
		},
	}
}

type patchRestartHandler struct {
	patchId string
}

func (p *patchRestartHandler) Handler() RequestHandler {
	return &patchRestartHandler{}
}

func (p *patchRestartHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	p.patchId = gimlet.GetVars(r)["patch_id"]
	return nil
}

func (p *patchRestartHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {

	// If the version has not been finalized, returns NotFound
	usr := MustHaveUser(ctx)
	err := sc.RestartVersion(p.patchId, usr.Id)
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "Restart error")
	}

	foundPatch, err := sc.FindPatchById(p.patchId)
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "Database error")
	}
	patchModel := &model.APIPatch{}
	err = patchModel.BuildFromService(*foundPatch)

	if err != nil {
		return ResponseData{}, errors.Wrap(err, "API model error")
	}

	return ResponseData{
		Result: []model.Model{patchModel},
	}, nil
}
