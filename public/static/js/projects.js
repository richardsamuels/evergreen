mciModule.controller('ProjectCtrl', function($scope, $window, $http, $location, $mdDialog) {

  $scope.availableTriggers = $window.availableTriggers
  $scope.userId = $window.user.Id;
  $scope.isAdmin = $window.isSuperUser || $window.isAdmin;
  $scope.isSuperUser = $window.isSuperUser;


  $scope.projectVars = {};
  $scope.patchVariants = [];
  $scope.projectRef = {};
  $scope.displayName = "";

  $scope.projectView = false;

  $scope.settingsFormData = {};
  $scope.saveMessage = "";

  $scope.modalTitle = 'New Project';
  $scope.modalOpen = false;
  $scope.newProject = {};
  $scope.newProjectMessage="";

  $scope.isDirty = false;
  $scope.triggers = [
    {
      trigger: "outcome",
      resource_type: "VERSION",
      label: "any version finishes",
    },
    {
      trigger: "failure",
      resource_type: "VERSION",
      label: "any version fails",
    },
    {
      trigger: "outcome",
      resource_type: "BUILD",
      label: "any build finishes",
      regex_selectors: buildRegexSelectors(),
    },
    {
      trigger: "failure",
      resource_type: "BUILD",
      label: "any build fails",
      regex_selectors: buildRegexSelectors(),
    },
    {
      trigger: "outcome",
      resource_type: "TASK",
      label: "any task finishes",
      regex_selectors: taskRegexSelectors(),
    },
    {
      trigger: "failure",
      resource_type: "TASK",
      label: "any task fails",
      regex_selectors: taskRegexSelectors(),
    },
    {
      trigger: "first-failure-in-version",
      resource_type: "TASK",
      label: "the first task failure occurs",
      regex_selectors: taskRegexSelectors(),
    },
    {
      trigger: "first-failure-in-build",
      resource_type: "TASK",
      label: "the first failure in each build occurs",
      regex_selectors: taskRegexSelectors(),
    },
    {
      trigger: "first-failure-in-version-with-name",
      resource_type: "TASK",
      label: "the first failure in each version for each task name occurs",
      regex_selectors: taskRegexSelectors(),
    },
    {
      trigger: "regression",
      resource_type: "TASK",
      label: "a previously passing task fails",
      regex_selectors: taskRegexSelectors(),
    },
    {
      trigger: "regression-by-test",
      resource_type: "TASK",
      label: "a previously passing test in a task fails",
      regex_selectors: taskRegexSelectors(),
    },
  ];

  // refreshTrackedProjects will populate the list of projects that should be displayed
  // depending on the user.
  $scope.refreshTrackedProjects = function(trackedProjects) {
    $scope.trackedProjects = trackedProjects
    $scope.enabledProjects = _.filter($scope.trackedProjects, _.property('enabled'));
    $scope.disabledProjects = _.filter($scope.trackedProjects, _.negate(_.property('enabled')));
  };


  $scope.refreshTrackedProjects($window.allTrackedProjects);

  $scope.showProject = function(project) {
    return !(project.length == 0);
  }

  $scope.isBatchTimeValid = function(t){
    if(t==''){
      return true
    }
    return !isNaN(Number(t)) && Number(t) >= 0
  }

  // TODO: EVG-3408
  $scope.isValidAlertDefinition = function(spec) {
    if (!spec) { return false; }
    if (spec.startsWith("JIRA:") && spec.split(":").length < 3) {
        return false
    }
    if (spec.startsWith("SLACK:") && spec.split(":").length < 2) {
        return false
    }
    return true
  }

  // TODO: EVG-3408
  $scope.addAlert = function(obj, trigger){
    if(!$scope.settingsFormData.alert_config) {
      $scope.settingsFormData.alert_config = {}
    }
    if(!$scope.settingsFormData.alert_config[trigger.id]){
      $scope.settingsFormData.alert_config[trigger.id] = []
    }
    $scope.settingsFormData.alert_config[trigger.id].push(NewAlert(obj.email))
    obj.editing = false
  }

  // TODO: EVG-3408
  $scope.getProjectAlertConfig = function(t){
    if(!$scope.settingsFormData.alert_config || !$scope.settingsFormData.alert_config[t]){
      return []
    }
    return $scope.settingsFormData.alert_config[t]
  }

  $scope.findProject = function(identifier){
    return _.find($scope.trackedProjects, function(project){
      return project.identifier == identifier;
    })
  }

  $scope.openAlertModal = function(){
    var modal = $('#alert-modal').modal('show');
  }

  $scope.openAdminModal = function(opt) {
    var modal = $('#admin-modal').modal('show');
    $scope.newProjectMessage = "";
    if (opt === 'newProject') {
      $('#project-name').focus();
      modal.on('shown.bs.modal', function() {
          $scope.modalOpen = true;
      });

      modal.on('hide.bs.modal', function() {
        $scope.modalOpen = false;
      });
    }
  };

  // addAdmin adds an admin name to the settingsFormData's list of admins
  $scope.addAdmin = function(){
    $scope.settingsFormData.admins.push($scope.admin_name);
    $scope.admin_name = "";
  }

  // removeAdmin removes the username located at index
  $scope.removeAdmin = function(index){
    $scope.settingsFormData.admins.splice(index, 1);
    $scope.isDirty = true;
  }


  $scope.addProject = function() {
    $scope.modalOpen = false;
    $('#admin-modal').modal('hide');

    if ($scope.findProject($scope.newProject.identifier)) {
      console.log("project identifier already exists");
      $scope.newProjectMessage = "Project name already exists.";
      $scope.newProject = {};
      return;
    }

    // if copyProject is set, copy the current project
    if ($scope.newProject.copyProject) {
      $scope.settingsFormData.batch_time = parseInt($scope.settingsFormData.batch_time);
      $http.put('/project/' + $scope.newProject.identifier, $scope.newProject).then(
        function(resp) {
          var data_put = resp.data;
          item = Object.assign({}, $scope.settingsFormData);
          item.setup_github_hook = false;
          item.pr_testing_enabled = false;
          item.enabled = false;
          $http.post('/project/' + $scope.newProject.identifier, item).then(
            function(resp) {
              $scope.refreshTrackedProjects(data_put.AllProjects);
              $scope.loadProject(data_put.ProjectId);
              $scope.newProject = {};
            },
            function(resp) {
              console.log("error saving data for new project: " + resp.status);
            });
        },
        function(resp) {
          console.log("error creating new project: " + resp.status);
        });

    // otherwise, create a blank project
    } else {
      $http.put('/project/' + $scope.newProject.identifier, $scope.newProject).then(
        function(resp) {
          var data = resp.data;
          $scope.refreshTrackedProjects(data.AllProjects);
          $scope.loadProject(data.ProjectId);
          $scope.newProject = {};
          $scope.settingsFormData.setup_github_hook = true;
          $scope.settingsFormData.tracks_push_events = true;
        },
        function(resp){
          console.log("error creating project: " + resp.status);
        });
    }
  };

  $scope.loadProject = function(projectId) {
    $http.get('/project/' + projectId).then(
      function(resp){
        var data = resp.data;
        $scope.projectView = true;
        $scope.projectRef = data.ProjectRef;
        if(data.ProjectVars === null) {
            data.ProjectVars = {}
        }
        if(data.ProjectRef === null) {
            data.ProjectRef = {}
        }
        $scope.projectVars = data.ProjectVars.vars || {};
        $scope.privateVars = data.ProjectVars.private_vars || {};
        $scope.githubHookID = data.github_hook.hook_id || 0;
        $scope.prTestingConflicts = data.pr_testing_conflicting_refs || [];
        $scope.prTestingEnabled = data.ProjectRef.pr_testing_enabled || false;

        $scope.aliases = data.aliases || [];
        $scope.aliases = _.sortBy($scope.aliases, function(v) {
          return v.alias + v.variant + v.task;
        });

        $scope.github_aliases = _.filter($scope.aliases, function(patch) {
          return patch.alias == "__github";
        });
        $scope.patch_aliases = _.filter($scope.aliases, function(patch) {
          return patch.alias != "__github";
        });
        for (var i = 0; i < $scope.patch_aliases.length; i++) {
          var alias = $scope.patch_aliases[i];
          if (alias.tags) {
            alias.tags_temp = alias.tags.join(',');
          }
        }

        $scope.settingsFormData = {
          identifier : $scope.projectRef.identifier,
          project_vars: $scope.projectVars,
          private_vars: $scope.privateVars,
          display_name : $scope.projectRef.display_name,
          remote_path:$scope.projectRef.remote_path,
          batch_time: parseInt($scope.projectRef.batch_time),
          deactivate_previous: $scope.projectRef.deactivate_previous,
          relative_url: $scope.projectRef.relative_url,
          branch_name: $scope.projectRef.branch_name || "master",
          owner_name: $scope.projectRef.owner_name,
          repo_name: $scope.projectRef.repo_name,
          enabled: $scope.projectRef.enabled,
          private: $scope.projectRef.private,
          patching_disabled: $scope.projectRef.patching_disabled,
          alert_config: $scope.projectRef.alert_config || {},
          repotracker_error: $scope.projectRef.repotracker_error || {},
          admins : $scope.projectRef.admins || [],
          setup_github_hook: $scope.githubHookID != 0,
          tracks_push_events: data.ProjectRef.tracks_push_events || false,
          pr_testing_enabled: data.ProjectRef.pr_testing_enabled || false,
          notify_on_failure: $scope.projectRef.notify_on_failure,
          force_repotracker_run: false,
          delete_aliases: [],
          delete_subscriptions: [],
        };

        $scope.subscriptions = _.map(data.subscriptions || [], function(v) {
          t = lookupTrigger($scope.triggers, v.trigger, v.resource_type);
          v.trigger_label = t.label;
          v.subscriber.label = subscriberLabel(v.subscriber);
          return v;
        });

        $scope.displayName = $scope.projectRef.display_name ? $scope.projectRef.display_name : $scope.projectRef.identifier;
        $location.hash($scope.projectRef.identifier);
        $scope.$emit('loadProject', $scope.projectRef.identifier, $scope.displayName);
      },
      function(resp) {
        console.log(resp.status);
      });
  };

  $scope.$on('$locationChangeStart', function(event) {
    $scope.hashLoad();
  });

  $scope.hashLoad = function() {
    var projectHash = $location.hash();
    if (projectHash) {
      // If the project in the hash exists and is not the current project, load it.
      if ( _.contains(_.pluck($scope.trackedProjects, "identifier"), projectHash) && ($scope.projectRef.identifier != projectHash)) {
        $scope.loadProject(projectHash);
      }
    }
  }

  $scope.valueString = function(name,value) {
    if($scope.privateVars[name]){
      return '{REDACTED}';
    }
    return value;
  }

  $scope.shouldHighlight = function(project) {
    if ($scope.projectRef) {
      return project.identifier == $scope.projectRef.identifier;
    }
    return false;
  }

  $scope.saveProject = function() {
    $scope.settingsFormData.batch_time = parseInt($scope.settingsFormData.batch_time);
    if ($scope.proj_var) {
      $scope.addProjectVar();
    }
    if ($scope.github_alias) {
      $scope.addGithubAlias();
      if($scope.github_alias) {
        if($scope.github_alias.variant) {
          $scope.invalidPatchDefinitionMessage = "Missing task regex";
        }else if($scope.github_alias_task) {
          $scope.invalidPatchDefinitionMessage = "Missing variant regex";
        }
        return;
      }
    }
    if ($scope.patch_alias) {
      $scope.addPatchAlias();
    }
    for (var i = 0; i < $scope.patch_aliases.length; i++) {
      var alias = $scope.patch_aliases[i];
      if (alias.tags_temp) {
        alias.tags = alias.tags_temp.split(',');
      }
    }

    $scope.settingsFormData.subscriptions = _.filter($scope.subscriptions, function(d) {
        return d.changed;
    });

    $scope.settingsFormData.project_aliases = $scope.github_aliases.concat($scope.patch_aliases);
    if ($scope.admin_name) {
      $scope.addAdmin();
    }
    $http.post('/project/' + $scope.settingsFormData.identifier, $scope.settingsFormData).then(
      function(resp) {
        var data = resp.data;
        $scope.saveMessage = "Settings Saved.";
        $scope.refreshTrackedProjects(data.AllProjects);
        $scope.settingsForm.$setPristine();
        $scope.settingsFormData.force_repotracker_run = false;
        $scope.loadProject($scope.settingsFormData.identifier)
        $scope.isDirty = false;
      },
      function(resp) {
        $scope.saveMessage = "Couldn't save project: " + resp.data.error;
        console.log(resp.status);
      });
  };

  $scope.addProjectVar = function() {
    if ($scope.proj_var.name && $scope.proj_var.value) {
      $scope.settingsFormData.project_vars[$scope.proj_var.name] = $scope.proj_var.value;
      if ($scope.proj_var.is_private) {
       $scope.settingsFormData.private_vars[$scope.proj_var.name] = true;
      }

      $scope.proj_var.name="";
      $scope.proj_var.value="";
      $scope.proj_var.is_private=false;
    }
  };

  $scope.addGithubAlias = function() {
    if ($scope.github_alias.variant && $scope.github_alias.task) {
      item = Object.assign({}, $scope.github_alias)
      item["alias"] = "__github"
      $scope.github_aliases = $scope.github_aliases.concat([item]);
      delete $scope.github_alias
      $scope.invalidPatchDefinitionMessage = "";
    }
  };

  $scope.addPatchAlias = function() {
    if ($scope.patch_alias.alias && $scope.patch_alias.variant && ($scope.patch_alias.task || $scope.patch_alias.tags_temp)) {
      item = Object.assign({}, $scope.patch_alias)
      if ($scope.patch_alias.tags) {
        item.tags = $scope.patch_alias.tags.split(',');
      }
      $scope.patch_aliases = $scope.patch_aliases.concat([item]);
      delete $scope.patch_alias
    }
  };

  $scope.removeProjectVar = function(name) {
    delete $scope.settingsFormData.project_vars[name];
    delete $scope.settingsFormData.private_vars[name];
    $scope.isDirty = true;
  };

  $scope.removeGithubAlias = function(i) {
    if ($scope.github_aliases[i]["_id"]) {
      $scope.settingsFormData.delete_aliases = $scope.settingsFormData.delete_aliases.concat([$scope.github_aliases[i]["_id"]])
    }
    $scope.github_aliases.splice(i, 1);
    $scope.isDirty = true;
  };

  $scope.removePatchAlias = function(i) {
    if ($scope.patch_aliases[i]["_id"]) {
      $scope.settingsFormData.delete_aliases = $scope.settingsFormData.delete_aliases.concat([$scope.patch_aliases[i]["_id"]])
    }
    $scope.patch_aliases.splice(i, 1);
    $scope.isDirty = true;
  };

  $scope.$watch("settingsForm.$dirty", function(dirty) {
    if (dirty){
      $scope.saveMessage = "You have unsaved changes.";
      $scope.isDirty = true;
    }
  });

  $scope.getAlertDisplay =function(alertObj){
    if(alertObj.provider=='email'){
      return "Send an e-mail to " + alertObj.settings.recipient
    }
    if (alertObj.provider=='jira'){
      return "File a "+alertObj.settings.issue+" JIRA ticket in "+ alertObj.settings.project
    }
    if (alertObj.provider=='slack'){
      return "Send a slack message to "+alertObj.settings.channel
    }
    return 'unknown'
  }

  $scope.removeAlert = function(triggerId, index){
    $scope.settingsFormData.alert_config[triggerId].splice(index, 1)
    $scope.isDirty = true;
  }

  $scope.isValidMergeBaseRevision = function(revision){
    return revision && revision.length >= 40;
  }

  $scope.setLastRevision = function() {
    if ($scope.settingsFormData.repotracker_error.exists) {
      var revisionUrl = '/project/' + $scope.settingsFormData.identifier + "/repo_revision";
      if (!$scope.isValidMergeBaseRevision($scope.settingsFormData.repotracker_error.merge_base_revision)){
        console.log("bad revision");
        return;
      }
      $http.put(revisionUrl, $scope.settingsFormData.repotracker_error.merge_base_revision).then(
        function(data) {
          $scope.settingsFormData.repotracker_error.exists = false;
        },
        function(resp){
          console.log(resp.status);
        });
    }
  }

  $scope.validKeyValue = function(keyName, value){
    if (!(keyName)){
        $scope.invalidKeyMessage = "";
      return false;
    }

    if (keyName.indexOf(".") != -1){
      $scope.invalidKeyMessage = "Project variable keys cannot have dots in them";
      return false;
    };

    if (!(value)){
      $scope.invalidKeyMessage = "";
      return false;
    }
    return true;
  }

  $scope.validPatchDefinition = function(variantRegex, taskRegex){
    if (!variantRegex || !taskRegex){
      return false;
    }

    return true;
  };

  $scope.validPatchAlias = function(alias, variantRegex, taskRegex, taskTags){
    if (!alias || !variantRegex || (!taskRegex && !taskTags)){
      return false;
    }

    return true;
  };

  $scope.addSubscription = function() {
      promise = addSubscriber($mdDialog, $scope.triggers);

      $mdDialog.show(promise).then(function(data){
          data.changed = true;
          $scope.isDirty = true;
          $scope.subscriptions.push(data);
      });
  };

  $scope.editSubscription = function(index) {
      promise = editSubscriber($mdDialog, $scope.triggers, $scope.subscriptions[index]);

      $mdDialog.show(promise).then(function(data){
          data.changed = true;
          $scope.isDirty = true;
          $scope.subscriptions[index] = data;
      });
  };

  $scope.removeSubscription = function(index) {
      if ($scope.subscriptions[index] && $scope.subscriptions[index].id) {
          $scope.settingsFormData.delete_subscriptions.push($scope.subscriptions[index].id);
      }
      $scope.subscriptions = _.filter($scope.subscriptions, function(s, i) {
          return index !== i;
      });
      $scope.isDirty = true;
  };
  $scope.show_build_break = true;

});

mciModule.directive('adminNewProject', function() {
    return {
        restrict: 'E',
        template:
    '<div class="row">' +
      '<div class="col-lg-12">' +
        'Enter project name ' +
        '<form style="display: inline" ng-submit="addProject()">' +
          '<input type="text" id="project-name" placeholder="project name" ng-model="newProject.identifier">' +
          ' <input type="checkbox" id="copy-project" ng-model="newProject.copyProject">' +
        ' Duplicate current project' +
        '</form>' +
        '<button type="submit" class="btn btn-primary" style="float: right; margin-left: 10px;" ng-click="addProject()">Create Project</button>' +
      '</div>' +
    '</div>'
  };
});

// TODO: EVG-3408
mciModule.directive('adminNewAlert', function() {
    return {
      restrict: 'E',
      templateUrl:'/static/partials/alert_modal_form.html',
      link: function(scope, element, attrs){
        scope.availableTriggers= [
          {id:"task_failed", display:"Any task fails..."},
          {id:"first_task_failed", display:"The first failure in a version occurs..."},
          {id:"task_fail_transition", display:"A task that had passed in a previous run fails"},
        ]
        scope.availableActions= [
          {id:"email", display:"Send an e-mail"},
        ]
        scope.setTrigger = function(index){
          scope.currentTrigger = scope.availableTriggers[index]
        }
        scope.currentTrigger = scope.availableTriggers[0]
      }
  };
});
