package lang

// Rails is the Ruby on Rails ecosystem's conventions, and that of Ruby projects
// following its layout.
//
// The shapes here were wrong in a way that took a field trial to find. `init`
// originally looked for `app/services/*/` and `app/jobs/*/`, which are a
// community pattern `rails new` never creates — so a canonical 600-file Rails
// app matched only `lib/*/`, covering 27 files, and reached a permanently green
// check over 4% of itself.
//
// Rails' actual convention is one directory per *layer* under `app/`, with flat
// files inside: app/models, app/controllers, app/agents. That is `app/*/`, and
// it goes last so the more specific shapes claim their directories first when a
// repo does use them.
var Rails = Lang{
	Name:    "Ruby on Rails",
	Markers: []string{"Gemfile", "Gemfile.lock", "config/application.rb"},
	Discover: []string{
		// Specific first: a repo that has adopted the service-object pattern
		// means those directories, not the layers around them.
		"app/services/*/",
		"app/jobs/*/",
		"services/*/",
		// Shared layers. These usually resolve into `shared:` rather than into
		// nodes, which is exactly why proposing them is worth it: an entry in
		// `shared:` is an accountable exemption, and code in neither place is
		// code nobody has decided about.
		"lib/*/",
		// The framework's own shape, last for the reason above.
		"app/*/",
	},
	TestGlobs: []string{"**/*_spec.rb", "**/*_test.rb"},
	// Rails directory names are already domain nouns, so the prefix-by-kind
	// rule reads naturally: app/services/billing -> svc_billing.
	Prefixes: map[string]string{
		"services": "svc_",
		"service":  "svc_",
		"adapters": "adp_",
		"adapter":  "adp_",
		"jobs":     "job_",
		"job":      "job_",
		"workers":  "job_",
	},
}
