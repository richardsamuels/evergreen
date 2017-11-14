package patch

import (
	"strings"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

const (
	// IntentCollection is the database collection that stores patch intents.
	IntentCollection = "patch_intents"

	// GithubIntentType represents patch intents created for GitHub.
	GithubIntentType = "github"
)

// Intent represents an intent to create a patch build and is processed by an amboy queue.
type Intent interface {
	// Insert inserts a patch intent in the database.
	Insert() error

	// SetProcessed should be called by an amboy queue after creating a patch from an intent.
	SetProcessed() error

	// IsProcessed returns whether a patch exists for this intent.
	IsProcessed() bool

	// GetType returns the patch intent, e.g., GithubType.
	GetType() string
}

// githubIntent represents an intent to create a patch build as a result of a
// PullRequestEvent webhook. These intents are processed asynchronously by an
// amboy queue.
type githubIntent struct {
	// ID is from the unique message id from the X-GitHub-Delivery header
	Id string `bson:"_id"`

	// Full Repository name, ex: mongodb/mongo
	RepoName string `bson:"repo_name"`

	// Pull request number for the project in GitHub.
	PRNumber int `bson:"pr_number"`

	// Github user that created the pull request
	User string `bson:"user"`

	// BaseHash is the base hash of the patch.
	BaseHash string `bson:"base_hash"`

	// URL is the URL of the patch in GitHub.
	URL string `bson:"url"`

	// Processed indicates whether a patch intent has been processed by the amboy queue.
	Processed bool `bson:"processed"`

	// IntentType indicates the type of the patch intent, i.e., GithubIntentType
	IntentType string `bson:"intent_type"`
}

// BSON fields for the patches
var (
	idKey         = bsonutil.MustHaveTag(githubIntent{}, "Id")
	repoNameKey   = bsonutil.MustHaveTag(githubIntent{}, "RepoName")
	prNumberKey   = bsonutil.MustHaveTag(githubIntent{}, "PRNumber")
	userKey       = bsonutil.MustHaveTag(githubIntent{}, "User")
	baseHashKey   = bsonutil.MustHaveTag(githubIntent{}, "BaseHash")
	urlKey        = bsonutil.MustHaveTag(githubIntent{}, "URL")
	processedKey  = bsonutil.MustHaveTag(githubIntent{}, "Processed")
	intentTypeKey = bsonutil.MustHaveTag(githubIntent{}, "IntentType")
)

// NewGithubIntent return a new github patch intent.
func NewGithubIntent(msgDeliveryId, repoName string, prNumber int, user, baseHash, url string) (Intent, error) {
	if msgDeliveryId == "" {
		return nil, errors.New("Unique msg id cannot be empty")
	}
	if repoName == "" || len(strings.Split(repoName, "/")) != 2 {
		return nil, errors.New("Repo name is invalid")
	}
	if prNumber == 0 {
		return nil, errors.New("PR number must not be 0")
	}
	if user == "" {
		return nil, errors.New("Github user name must not be empty string")
	}
	if len(baseHash) == 0 {
		return nil, errors.New("Base hash must not be empty")
	}
	if !strings.HasPrefix(url, "http") {
		return nil, errors.Errorf("URL does not appear valid (%s)", url)
	}

	return &githubIntent{
		Id:         msgDeliveryId,
		RepoName:   repoName,
		PRNumber:   prNumber,
		User:       user,
		BaseHash:   baseHash,
		URL:        url,
		IntentType: GithubIntentType,
	}, nil
}

// SetProcessed should be called by an amboy queue after creating a patch from an intent.
func (g *githubIntent) SetProcessed() error {
	g.Processed = true
	return updateOneIntent(
		bson.M{idKey: g.Id},
		bson.M{"$set": bson.M{processedKey: g.Processed}},
	)
}

// updateOne updates one patch intent.
func updateOneIntent(query interface{}, update interface{}) error {
	return db.Update(
		IntentCollection,
		query,
		update,
	)
}

// IsProcessed returns whether a patch exists for this intent.
func (g *githubIntent) IsProcessed() bool {
	return g.Processed
}

// GetType returns the patch intent, e.g., GithubIntentType.
func (g *githubIntent) GetType() string {
	return g.IntentType
}

// Insert inserts a patch intent in the database.
func (g *githubIntent) Insert() error {
	return db.Insert(IntentCollection, g)
}

// FindUnprocessedGithubIntents finds all patch intents that have not yet been processed.
func FindUnprocessedGithubIntents() ([]*githubIntent, error) {
	var intents []*githubIntent
	err := db.FindAllQ(IntentCollection, db.Query(bson.M{processedKey: false, intentTypeKey: GithubIntentType}), &intents)
	if err != nil {
		return []*githubIntent{}, err
	}
	return intents, nil
}
