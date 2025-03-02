package reconcilers

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	runtimev1 "github.com/rilldata/rill/proto/gen/rill/runtime/v1"
	"github.com/rilldata/rill/runtime"
	compilerv1 "github.com/rilldata/rill/runtime/compilers/rillv1"
	"github.com/rilldata/rill/runtime/drivers"
	"golang.org/x/exp/slog"
	"google.golang.org/protobuf/proto"
)

var ErrParserHasParseErrors = errors.New("encountered parse errors")

func init() {
	runtime.RegisterReconcilerInitializer(runtime.ResourceKindProjectParser, newProjectParser)
}

type ProjectParserReconciler struct {
	C *runtime.Controller
}

func newProjectParser(c *runtime.Controller) runtime.Reconciler {
	return &ProjectParserReconciler{C: c}
}

func (r *ProjectParserReconciler) Close(ctx context.Context) error {
	return nil
}

func (r *ProjectParserReconciler) AssignSpec(from, to *runtimev1.Resource) error {
	a := from.GetProjectParser()
	b := to.GetProjectParser()
	if a == nil || b == nil {
		return fmt.Errorf("cannot assign spec from %T to %T", from.Resource, to.Resource)
	}
	b.Spec = a.Spec
	return nil
}

func (r *ProjectParserReconciler) AssignState(from, to *runtimev1.Resource) error {
	a := from.GetProjectParser()
	b := to.GetProjectParser()
	if a == nil || b == nil {
		return fmt.Errorf("cannot assign state from %T to %T", from.Resource, to.Resource)
	}
	b.State = a.State
	return nil
}

func (r *ProjectParserReconciler) ResetState(res *runtimev1.Resource) error {
	res.GetProjectParser().State = &runtimev1.ProjectParserState{}
	return nil
}

func (r *ProjectParserReconciler) Reconcile(ctx context.Context, n *runtimev1.ResourceName) runtime.ReconcileResult {
	// Get ProjectParser resource
	self, err := r.C.Get(ctx, n, true)
	if err != nil {
		return runtime.ReconcileResult{Err: err}
	}
	pp := self.GetProjectParser()
	if pp == nil {
		return runtime.ReconcileResult{Err: errors.New("not a project parser")}
	}

	// Reset watching to false (in case of a crash during a previous watch)
	if pp.State.Watching {
		pp.State.Watching = false
		if err = r.C.UpdateState(ctx, n, self); err != nil {
			return runtime.ReconcileResult{Err: err}
		}
	}

	// Does not support renames
	if self.Meta.RenamedFrom != nil {
		return runtime.ReconcileResult{Err: fmt.Errorf("project parser cannot be renamed")}
	}

	// If deleted, remove all resources created by self
	if self.Meta.DeletedOn != nil {
		r.C.Lock(ctx)
		defer r.C.Unlock(ctx)

		resources, err := r.C.List(ctx, "", false)
		if err != nil {
			return runtime.ReconcileResult{Err: err}
		}

		for _, resource := range resources {
			if equalResourceName(resource.Meta.Owner, self.Meta.Name) {
				err := r.C.Delete(ctx, resource.Meta.Name)
				if err != nil {
					return runtime.ReconcileResult{Err: err}
				}
			}
		}

		return runtime.ReconcileResult{}
	}

	// Get and sync repo
	repo, release, err := r.C.Runtime.Repo(ctx, r.C.InstanceID)
	if err != nil {
		return runtime.ReconcileResult{Err: fmt.Errorf("failed to access repo: %w", err)}
	}
	defer release()
	err = repo.Sync(ctx)
	if err != nil {
		return runtime.ReconcileResult{Err: fmt.Errorf("failed to sync repo: %w", err)}
	}

	// Update commit sha
	hash, err := repo.CommitHash(ctx)
	if err != nil {
		// Not worth failing the reconcile for this. On error, it'll just set CurrentCommitSha to "".
		r.C.Logger.Error("failed to get commit hash", slog.String("err", err.Error()))
	}
	if pp.State.CurrentCommitSha != hash {
		pp.State.CurrentCommitSha = hash
		err = r.C.UpdateState(ctx, n, self)
		if err != nil {
			return runtime.ReconcileResult{Err: err}
		}
	}

	// Get instance
	inst, err := r.C.Runtime.Instance(ctx, r.C.InstanceID)
	if err != nil {
		return runtime.ReconcileResult{Err: fmt.Errorf("failed to find instance: %w", err)}
	}

	// Find DuckDB connectors
	var duckdbConnectors []string
	for _, connector := range inst.Connectors {
		if connector.Type == "duckdb" {
			duckdbConnectors = append(duckdbConnectors, connector.Name)
		}
	}

	// Parse the project
	parser, err := compilerv1.Parse(ctx, repo, r.C.InstanceID, inst.OLAPConnector, duckdbConnectors)
	if err != nil {
		return runtime.ReconcileResult{Err: fmt.Errorf("failed to parse: %w", err)}
	}

	// Do the actual reconciliation of parsed resources and catalog resources
	err = r.reconcileParser(ctx, inst, self, parser, nil, nil)

	// If err is not for parse errors, always return. Otherwise, only return it if we're not watching for changes.
	if err != nil && !errors.Is(err, ErrParserHasParseErrors) {
		return runtime.ReconcileResult{Err: err}
	}
	if !inst.WatchRepo {
		return runtime.ReconcileResult{Err: err}
	}

	// Set watching to true and add a defer to ensure it's set to false on exit
	pp.State.Watching = true
	if err = r.C.UpdateState(ctx, n, self); err != nil {
		return runtime.ReconcileResult{Err: err}
	}
	defer func() {
		pp.State.Watching = false
		if err = r.C.UpdateState(ctx, n, self); err != nil {
			r.C.Logger.Error("failed to update watch state", slog.Any("error", err))
		}
	}()

	// Start a watcher that incrementally reparses the project.
	// This is a blocking and long-running call, which is supported by the controller.
	// If pp.Spec is changed, the controller will cancel the context and call Reconcile again.
	var reparseErr error
	ctx, cancel := context.WithCancel(ctx)
	err = repo.Watch(ctx, func(events []drivers.WatchEvent) {
		// Get changed paths that are not directories
		changedPaths := make([]string, 0, len(events))
		for _, e := range events {
			if e.Dir {
				continue
			}
			if strings.HasSuffix(e.Path, ".db") || strings.HasSuffix(e.Path, ".wal") {
				continue
			}
			changedPaths = append(changedPaths, e.Path)
		}

		if len(changedPaths) == 0 {
			return
		}

		// If reparsing fails, we cancel repo.Watch and exit.
		// NOTE: Parse errors are not returned here (they're stored in p.Errors). Errors returned from Reparse are mainly file system errors.
		diff, err := parser.Reparse(ctx, changedPaths)
		if err == nil {
			err = r.reconcileParser(ctx, inst, self, parser, diff, changedPaths)
		}
		if err != nil && !errors.Is(err, ErrParserHasParseErrors) {
			reparseErr = err
			cancel()
			return
		}
	})
	if reparseErr != nil {
		err = fmt.Errorf("re-parse failed: %w", reparseErr)
	} else if err != nil {
		if errors.Is(err, ctx.Err()) {
			// The controller cancelled the context. It means pp.Spec was changed. Will be rescheduled.
			return runtime.ReconcileResult{Err: err}
		}
		err = fmt.Errorf("watch failed: %w", err)
	}

	// If the watch failed, we return without rescheduling.
	// TODO: Should we have some kind of retry?
	r.C.Logger.Error("Stopped watching for file changes", slog.String("err", err.Error()))
	return runtime.ReconcileResult{Err: err}
}

// reconcileParser reconciles a parser's output with the current resources in the catalog.
func (r *ProjectParserReconciler) reconcileParser(ctx context.Context, inst *drivers.Instance, self *runtimev1.Resource, parser *compilerv1.Parser, diff *compilerv1.Diff, changedPaths []string) error {
	// Update parse errors
	pp := self.GetProjectParser()
	pp.State.ParseErrors = parser.Errors
	err := r.C.UpdateState(ctx, self.Meta.Name, self)
	if err != nil {
		return err
	}

	// Log parse errors
	if diff == nil {
		// This handles a very specific case - when opening the application on an uninitialized directory, we do not want to print an error for "rill.yaml not found".
		// But if the user subsequently in the session, after initializing the project, breaks rill.yaml, then we DO want to log the error.
		// So we rely on StateVersion == 1 on the first call to the reconciler.
		// (The UpdateState calls above do not mutate `self`, which is a cloned object, so the starting StateVersion is preserved here. Also quite hacky.)
		skipRillYAMLErr := inst.IgnoreInitialInvalidProjectError && self.Meta.StateVersion == 1

		for _, e := range parser.Errors {
			if skipRillYAMLErr && e.FilePath == "/rill.yaml" {
				continue
			}
			r.C.Logger.Warn("Parser error", slog.String("path", e.FilePath), slog.String("err", e.Message))
		}
	} else if diff.Skipped {
		r.C.Logger.Warn("Not parsing changed paths due to broken rill.yaml")
	} else {
		for _, e := range parser.Errors {
			if slices.Contains(changedPaths, e.FilePath) {
				r.C.Logger.Warn("Parser error", slog.String("path", e.FilePath), slog.String("err", e.Message))
			}
		}
	}

	// Set an error without returning to mark if there are parse errors (if not, force error to nil in case there previously were parse errors)
	var parseErrsErr error
	if len(parser.Errors) > 0 {
		parseErrsErr = ErrParserHasParseErrors
	}
	err = r.C.UpdateError(ctx, self.Meta.Name, parseErrsErr)
	if err != nil {
		return err
	}

	// If RillYAML is missing, don't reconcile anything
	if parser.RillYAML == nil {
		return parseErrsErr
	}

	// Treat reloads the same as a fresh parse (where there's no diff)
	if diff != nil && diff.Reloaded {
		diff = nil
	}

	// Update state from rill.yaml and .env
	if diff == nil || diff.ModifiedDotEnv {
		err := r.reconcileProjectConfig(ctx, parser)
		if err != nil {
			return err
		}
	}

	// Reconcile resources.
	// The lock serves to delay the controller from triggering reconciliation until all resources have been updated.
	// By delaying reconciliation until all resources have been updated, we don't need to worry about making changes in DAG order here.
	r.C.Lock(ctx)
	defer r.C.Unlock(ctx)
	if diff != nil {
		err = r.reconcileResourcesDiff(ctx, inst, self, parser, diff)
		if err != nil {
			return err
		}
		return parseErrsErr // Keep the parseErrsErr in this case
	}

	err = r.reconcileResources(ctx, inst, self, parser)
	if err != nil {
		return err
	}
	return parseErrsErr // Keep the parseErrsErr in this case
}

// reconcileProjectConfig updates instance config derived from rill.yaml and .env
func (r *ProjectParserReconciler) reconcileProjectConfig(ctx context.Context, parser *compilerv1.Parser) error {
	inst, err := r.C.Runtime.Instance(ctx, r.C.InstanceID)
	if err != nil {
		return err
	}

	// Shallow clone for editing
	tmp := *inst
	inst = &tmp

	conns := make([]*runtimev1.Connector, 0, len(parser.RillYAML.Connectors))
	for _, c := range parser.RillYAML.Connectors {
		conns = append(conns, &runtimev1.Connector{
			Type:   c.Type,
			Name:   c.Name,
			Config: c.Defaults,
		})
	}
	inst.ProjectConnectors = conns

	vars := make(map[string]string)
	for _, v := range parser.RillYAML.Variables {
		vars[v.Name] = v.Default
	}
	for k, v := range parser.DotEnv {
		vars[k] = v
	}
	inst.ProjectVariables = vars

	// TODO: Passing "false" guards against infinite cancellations and restarts of the controller,
	// but it also ignores potential consistency issues where we update connector config without evicting cached connctions,
	// or where we update variables and don't re-evaluate all resources.
	err = r.C.Runtime.EditInstance(ctx, inst, false)
	if err != nil {
		return err
	}

	return nil
}

// reconcileResources creates, updates and deletes resources as necessary to match the parser's output with the current resources in the catalog.
func (r *ProjectParserReconciler) reconcileResources(ctx context.Context, inst *drivers.Instance, self *runtimev1.Resource, parser *compilerv1.Parser) error {
	// Gather resources to delete so we can check for renames.
	var deleteResources []*runtimev1.Resource

	// Pass over all existing resources in the catalog.
	resources, err := r.C.List(ctx, "", false)
	if err != nil {
		return err
	}
	seen := make(map[compilerv1.ResourceName]bool, len(resources))
	for _, rr := range resources {
		// Skip if the resource was not created by the parser.
		// If a code file is added for a currently ad-hoc resource, the putParserResourceDef call for it will fail.
		if !equalResourceName(rr.Meta.Owner, self.Meta.Name) {
			continue
		}

		n := resourceNameToCompiler(rr.Meta.Name).Normalized()
		def, ok := parser.Resources[n]

		// If the existing resource is in the parser output, update it.
		// NOTE: putParserResourceDef renames if the casing of the name has changed.
		if ok {
			seen[n] = true
			err = r.putParserResourceDef(ctx, inst, self, def, rr)
			if err != nil {
				return err
			}
			continue
		}

		// If the existing resource is not in the parser output, delete it
		deleteResources = append(deleteResources, rr)
	}

	// Insert resources for the parser outputs that were not seen when passing over the existing resources
	for _, def := range parser.Resources {
		if seen[def.Name.Normalized()] {
			continue
		}

		// Rename if possible
		renamed := false
		for idx, rr := range deleteResources {
			if rr == nil {
				// Already renamed
				continue
			}
			renamed, err = r.attemptRename(ctx, inst, self, def, rr)
			if err != nil {
				return err
			}
			if renamed {
				deleteResources[idx] = nil
				break
			}
		}
		if renamed {
			continue
		}

		// Insert resource
		err = r.putParserResourceDef(ctx, inst, self, def, nil)
		if err != nil {
			return err
		}
	}

	// Delete resources that did not get renamed
	for _, rr := range deleteResources {
		// The ones that got renamed were set to nil
		if rr == nil {
			continue
		}

		err = r.C.Delete(ctx, rr.Meta.Name)
		if err != nil {
			return err
		}
	}

	return nil
}

// reconcileResourcesDiff is similar to reconcileResources, but uses a diff from parser.Reparse instead of doing a full comparison of all resources.
func (r *ProjectParserReconciler) reconcileResourcesDiff(ctx context.Context, inst *drivers.Instance, self *runtimev1.Resource, parser *compilerv1.Parser, diff *compilerv1.Diff) error {
	// Gather resource to delete so we can check for renames.
	deleteResources := make([]*runtimev1.Resource, 0, len(diff.Deleted))
	for _, n := range diff.Deleted {
		r, err := r.C.Get(ctx, resourceNameFromCompiler(n), false)
		if err != nil {
			return err
		}
		deleteResources = append(deleteResources, r)
	}

	// Updates
	for _, n := range diff.Modified {
		existing, err := r.C.Get(ctx, resourceNameFromCompiler(n), false)
		if err != nil {
			return err
		}
		def := parser.Resources[n.Normalized()]
		err = r.putParserResourceDef(ctx, inst, self, def, existing)
		if err != nil {
			return err
		}
	}

	// Inserts
	for _, n := range diff.Added {
		def := parser.Resources[n.Normalized()]

		// Rename if possible
		renamed := false
		for idx, rr := range deleteResources {
			var err error
			renamed, err = r.attemptRename(ctx, inst, self, def, rr)
			if err != nil {
				return err
			}
			if renamed {
				deleteResources[idx] = nil
				break
			}
		}
		if renamed {
			continue
		}

		// Insert resource
		err := r.putParserResourceDef(ctx, inst, self, def, nil)
		if err != nil {
			return err
		}
	}

	// Deletes
	for _, rr := range deleteResources {
		// The ones that got renamed were set to nil
		if rr == nil {
			continue
		}

		err := r.C.Delete(ctx, rr.Meta.Name)
		if err != nil {
			return err
		}
	}

	return nil
}

// putParserResourceDef creates or updates a resource in the catalog based on a parser resource definition.
// It does an insert if existing is nil, otherwise it does an update.
// If existing is not nil, it compares values and only updates meta/spec values if they have changed (ensuring stable resource version numbers).
func (r *ProjectParserReconciler) putParserResourceDef(ctx context.Context, inst *drivers.Instance, self *runtimev1.Resource, def *compilerv1.Resource, existing *runtimev1.Resource) error {
	// Apply defaults
	def = applySpecDefaults(inst, def)

	// Make resource spec to insert/update.
	// res should be nil if no spec changes are needed.
	var res *runtimev1.Resource
	switch def.Name.Kind {
	case compilerv1.ResourceKindSource:
		if existing == nil || !equalSourceSpec(existing.GetSource().Spec, def.SourceSpec) {
			res = &runtimev1.Resource{Resource: &runtimev1.Resource_Source{Source: &runtimev1.SourceV2{Spec: def.SourceSpec}}}
		}
	case compilerv1.ResourceKindModel:
		if existing == nil || !equalModelSpec(existing.GetModel().Spec, def.ModelSpec) {
			res = &runtimev1.Resource{Resource: &runtimev1.Resource_Model{Model: &runtimev1.ModelV2{Spec: def.ModelSpec}}}
		}
	case compilerv1.ResourceKindMetricsView:
		if existing == nil || !equalMetricsViewSpec(existing.GetMetricsView().Spec, def.MetricsViewSpec) {
			res = &runtimev1.Resource{Resource: &runtimev1.Resource_MetricsView{MetricsView: &runtimev1.MetricsViewV2{Spec: def.MetricsViewSpec}}}
		}
	case compilerv1.ResourceKindMigration:
		if existing == nil || !equalMigrationSpec(existing.GetMigration().Spec, def.MigrationSpec) {
			res = &runtimev1.Resource{Resource: &runtimev1.Resource_Migration{Migration: &runtimev1.Migration{Spec: def.MigrationSpec}}}
		}
	case compilerv1.ResourceKindReport:
		if existing == nil || !equalReportSpec(existing.GetReport().Spec, def.ReportSpec) {
			res = &runtimev1.Resource{Resource: &runtimev1.Resource_Report{Report: &runtimev1.Report{Spec: def.ReportSpec}}}
		}
	case compilerv1.ResourceKindTheme:
		if existing == nil || !equalThemeSpec(existing.GetTheme().Spec, def.ThemeSpec) {
			res = &runtimev1.Resource{Resource: &runtimev1.Resource_Theme{Theme: &runtimev1.Theme{Spec: def.ThemeSpec}}}
		}
	default:
		panic(fmt.Errorf("unknown resource kind %q", def.Name.Kind))
	}

	// Make refs for the resource meta
	refs := make([]*runtimev1.ResourceName, 0, len(def.Refs))
	for _, r := range def.Refs {
		refs = append(refs, resourceNameFromCompiler(r))
	}

	// Create and return if not updating
	n := resourceNameFromCompiler(def.Name)
	if existing == nil {
		return r.C.Create(ctx, n, refs, self.Meta.Name, def.Paths, false, res)
	}

	// The name may have changed to a different case (e.g. aAa -> Aaa)
	if n.Kind == existing.Meta.Name.Kind && n.Name != existing.Meta.Name.Name {
		err := r.C.UpdateName(ctx, existing.Meta.Name, n, self.Meta.Name, def.Paths)
		if err != nil {
			return err
		}
	}

	// Update meta if refs or file paths changed
	if !slices.Equal(existing.Meta.FilePaths, def.Paths) || !equalResourceNames(existing.Meta.Refs, refs) {
		err := r.C.UpdateMeta(ctx, n, refs, self.Meta.Name, def.Paths)
		if err != nil {
			return err
		}
	}

	// Update spec if it changed
	if res != nil {
		err := r.C.UpdateMeta(ctx, n, refs, self.Meta.Name, def.Paths)
		if err != nil {
			return err
		}
		err = r.C.UpdateSpec(ctx, n, res)
		if err != nil {
			return err
		}
	}

	return nil
}

// attemptRename renames an existing resource if its spec matches a parser resource definition.
// It returns false if no rename was done.
// In addition to renaming, it also updates the resource's meta to match the parser resource definition.
func (r *ProjectParserReconciler) attemptRename(ctx context.Context, inst *drivers.Instance, self *runtimev1.Resource, def *compilerv1.Resource, existing *runtimev1.Resource) (bool, error) {
	newName := resourceNameFromCompiler(def.Name)
	if existing.Meta.Name.Kind != newName.Kind {
		return false, nil
	}

	// Check refs are the same (note: refs are always sorted)
	if len(existing.Meta.Refs) != len(def.Refs) {
		return false, nil
	}
	for i, n := range existing.Meta.Refs {
		if resourceNameToCompiler(n) != def.Refs[i] {
			return false, nil
		}
	}

	// Apply defaults before comparing specs
	def = applySpecDefaults(inst, def)

	// Check spec is the same
	switch def.Name.Kind {
	case compilerv1.ResourceKindSource:
		if !equalSourceSpec(existing.GetSource().Spec, def.SourceSpec) {
			return false, nil
		}
	case compilerv1.ResourceKindModel:
		if !equalModelSpec(existing.GetModel().Spec, def.ModelSpec) {
			return false, nil
		}
	case compilerv1.ResourceKindMetricsView:
		if !equalMetricsViewSpec(existing.GetMetricsView().Spec, def.MetricsViewSpec) {
			return false, nil
		}
	case compilerv1.ResourceKindMigration:
		if !equalMigrationSpec(existing.GetMigration().Spec, def.MigrationSpec) {
			return false, nil
		}
	default:
		// NOTE: No panic because we don't need to support renames for all resource kinds.
		// If renaming is not supported, it will just do a delete + insert instead.
		return false, nil
	}

	// NOTE: Not comparing owner and paths since changing those are allowed when renaming.

	// Run rename
	err := r.C.UpdateName(ctx, existing.Meta.Name, newName, self.Meta.Name, def.Paths)
	if err != nil {
		return false, err
	}

	return true, nil
}

// applySpecDefaults applies instance-level default properties to a resource spec.
func applySpecDefaults(inst *drivers.Instance, def *compilerv1.Resource) *compilerv1.Resource {
	switch def.Name.Kind {
	case compilerv1.ResourceKindSource:
		def.SourceSpec.StageChanges = inst.StageChanges
	case compilerv1.ResourceKindModel:
		def.ModelSpec.StageChanges = inst.StageChanges
		if def.ModelSpec.Materialize == nil {
			def.ModelSpec.Materialize = &inst.ModelDefaultMaterialize
		}
		def.ModelSpec.MaterializeDelaySeconds = inst.ModelMaterializeDelaySeconds
	default:
		// Nothing to do
	}
	return def
}

func resourceNameFromCompiler(name compilerv1.ResourceName) *runtimev1.ResourceName {
	switch name.Kind {
	case compilerv1.ResourceKindSource:
		return &runtimev1.ResourceName{Kind: runtime.ResourceKindSource, Name: name.Name}
	case compilerv1.ResourceKindModel:
		return &runtimev1.ResourceName{Kind: runtime.ResourceKindModel, Name: name.Name}
	case compilerv1.ResourceKindMetricsView:
		return &runtimev1.ResourceName{Kind: runtime.ResourceKindMetricsView, Name: name.Name}
	case compilerv1.ResourceKindMigration:
		return &runtimev1.ResourceName{Kind: runtime.ResourceKindMigration, Name: name.Name}
	case compilerv1.ResourceKindReport:
		return &runtimev1.ResourceName{Kind: runtime.ResourceKindReport, Name: name.Name}
	case compilerv1.ResourceKindTheme:
		return &runtimev1.ResourceName{Kind: runtime.ResourceKindTheme, Name: name.Name}
	default:
		panic(fmt.Errorf("unknown resource kind %q", name.Kind))
	}
}

func resourceNameToCompiler(name *runtimev1.ResourceName) compilerv1.ResourceName {
	switch name.Kind {
	case runtime.ResourceKindSource:
		return compilerv1.ResourceName{Kind: compilerv1.ResourceKindSource, Name: name.Name}
	case runtime.ResourceKindModel:
		return compilerv1.ResourceName{Kind: compilerv1.ResourceKindModel, Name: name.Name}
	case runtime.ResourceKindMetricsView:
		return compilerv1.ResourceName{Kind: compilerv1.ResourceKindMetricsView, Name: name.Name}
	case runtime.ResourceKindMigration:
		return compilerv1.ResourceName{Kind: compilerv1.ResourceKindMigration, Name: name.Name}
	case runtime.ResourceKindReport:
		return compilerv1.ResourceName{Kind: compilerv1.ResourceKindReport, Name: name.Name}
	case runtime.ResourceKindTheme:
		return compilerv1.ResourceName{Kind: compilerv1.ResourceKindTheme, Name: name.Name}
	default:
		panic(fmt.Errorf("unknown resource kind %q", name.Kind))
	}
}

func equalResourceName(a, b *runtimev1.ResourceName) bool {
	return a != nil && b != nil && a.Kind == b.Kind && strings.EqualFold(a.Name, b.Name)
}

func equalResourceNames(a, b []*runtimev1.ResourceName) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if !equalResourceName(v, b[i]) {
			return false
		}
	}
	return true
}

func equalSourceSpec(a, b *runtimev1.SourceSpec) bool {
	return proto.Equal(a, b)
}

func equalModelSpec(a, b *runtimev1.ModelSpec) bool {
	return proto.Equal(a, b)
}

func equalMetricsViewSpec(a, b *runtimev1.MetricsViewSpec) bool {
	return proto.Equal(a, b)
}

func equalMigrationSpec(a, b *runtimev1.MigrationSpec) bool {
	return proto.Equal(a, b)
}

func equalReportSpec(a, b *runtimev1.ReportSpec) bool {
	return proto.Equal(a, b)
}

func equalThemeSpec(a, b *runtimev1.ThemeSpec) bool {
	return proto.Equal(a, b)
}
