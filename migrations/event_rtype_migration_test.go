package migrations

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/anser"
	anserdb "github.com/mongodb/anser/db"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

type eventRTypeMigrationSuite struct {
	env      *mock.Environment
	database string
	session  anserdb.Session
	cancel   func()

	events []anserdb.Document

	suite.Suite
}

func TestEventRTypeMigration(t *testing.T) {
	require := require.New(t)

	mgoSession, database, err := db.GetGlobalSessionFactory().GetSession()
	require.NoError(err)
	defer mgoSession.Close()

	session := anserdb.WrapSession(mgoSession.Copy())
	defer session.Close()

	ctx, cancel := context.WithCancel(context.Background())
	s := &eventRTypeMigrationSuite{
		env:      &mock.Environment{},
		session:  session,
		database: database.Name,
		cancel:   cancel,
	}

	require.NoError(s.env.Configure(ctx, filepath.Join(evergreen.FindEvergreenHome(), testutil.TestDir, testutil.TestSettings), nil))
	require.NoError(s.env.LocalQueue().Start(ctx))

	anser.ResetEnvironment()
	require.NoError(anser.GetEnvironment().Setup(s.env.LocalQueue(), s.session))
	anser.GetEnvironment().RegisterCloser(func() error { cancel(); return nil })

	suite.Run(t, s)
}

func (s *eventRTypeMigrationSuite) SetupTest() {
	loc, err := time.LoadLocation("UTC")
	s.NoError(err)
	date, err := time.ParseInLocation(time.RFC3339Nano, "2017-06-20T18:07:24.991Z", loc)
	s.NoError(err)

	s.NoError(db.ClearCollections(event.AllLogCollection))
	s.events = []anserdb.Document{
		anserdb.Document{
			"_id":    bson.ObjectIdHex("5949645c9acd9604fdd202d7"),
			"ts":     date,
			"r_id":   "macos.example.com",
			"e_type": "HOST_TASK_FINISHED",
			"data": anserdb.Document{
				"r_type": "HOST",
				"t_id":   "mci_osx_dist_165359be9d1ca311e964ebc4a50e66da42998e65_17_06_20_16_14_44",
				"t_st":   "success",
			},
		},
		anserdb.Document{
			"_id":    bson.ObjectIdHex("5949645c9acd9604fdd202d8"),
			"ts":     date,
			"r_id":   "macos.example.com",
			"e_type": "HOST_TASK_FINISHED",
			"r_type": "HOST",
			"data": anserdb.Document{
				"t_id": "mci_osx_dist_165359be9d1ca311e964ebc4a50e66da42998e65_17_06_20_16_14_44",
				"t_st": "success",
			},
		},
		anserdb.Document{
			"_id":    bson.ObjectIdHex("5949645c9acd9604fdd202d9"),
			"ts":     date,
			"r_id":   "macos.example.com",
			"e_type": "SOMETHING_AWESOME",
			"data": anserdb.Document{
				"r_type": "SOMETHINGELSE",
				"other":  "data",
			},
		},
	}
	for _, e := range s.events {
		s.NoError(db.Insert(event.AllLogCollection, e))
	}
}

func (s *eventRTypeMigrationSuite) TearDownSuite() {
	s.cancel()
}

func (s *eventRTypeMigrationSuite) TestMigration() {
	gen, err := makeEventRTypeMigration(event.AllLogCollection)(anser.GetEnvironment(), s.database, 50)
	s.Require().NoError(err)
	gen.Run()
	s.Require().NoError(gen.Error())

	i := 0
	for j := range gen.Jobs() {
		i++
		j.Run()
		s.NoError(j.Error())
	}
	s.Equal(2, i)

	out := []bson.M{}
	s.Require().NoError(db.FindAllQ(event.AllLogCollection, db.Q{}, &out))
	s.Len(out, 3)

	for _, e := range out {
		eventData, ok := e["data"]
		s.True(ok)

		eventDataBSON, ok := eventData.(bson.M)
		s.True(ok)

		if e["_id"].(bson.ObjectId).Hex() == "5949645c9acd9604fdd202d7" {
			s.Equal("HOST", e["r_type"])
			s.Equal("HOST_TASK_FINISHED", e["e_type"])

			s.Equal("mci_osx_dist_165359be9d1ca311e964ebc4a50e66da42998e65_17_06_20_16_14_44", eventDataBSON["t_id"])

		} else if e["_id"].(bson.ObjectId).Hex() == "5949645c9acd9604fdd202d8" {
			s.Equal("HOST", e["r_type"])
			s.Equal("HOST_TASK_FINISHED", e["e_type"])

			s.Equal("mci_osx_dist_165359be9d1ca311e964ebc4a50e66da42998e65_17_06_20_16_14_44", eventDataBSON["t_id"])

		} else if e["_id"].(bson.ObjectId).Hex() == "5949645c9acd9604fdd202d9" {
			s.Equal("SOMETHINGELSE", e["r_type"])
			s.Equal("SOMETHING_AWESOME", e["e_type"])

			s.Equal("data", eventDataBSON["other"])

		} else {
			s.T().Error("unknown object id")
		}

		_, ok = eventDataBSON["r_type"]
		s.False(ok)
	}
}
