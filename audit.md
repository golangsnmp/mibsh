# Mibsh Audit Report

## Critical (multiple agents flagged)

- [x] **Split app_update.go (1675 lines)** - Monolithic dispatcher mixing keyboard handling, mouse handling, SNMP operations, layout generation, and chord resolution. Recommended split: `app_keys.go`, `app_mouse.go`, `app_snmp.go`, `app_layout.go`, with a slimmed `app_update.go` containing only the top-level `Update()` dispatch.

- [x] **Rendering duplication in result_pane.go** - `renderTreeRowFn` (449-493) and `renderSelectedTreeRow` (496-569) duplicate leaf/branch rendering logic 3 times across focused/unfocused/unselected states. ~120 lines of near-identical code that could be a single parameterized function.

- [x] **View() vs snapshot() duplication (app_draw.go / snapshot.go)** - `snapshot()` (207 lines) duplicates nearly the entire `View()` rendering. Any change to `View()` must be manually mirrored. Should extract shared rendering into a common method, with `View()` and `snapshot()` as thin wrappers.

- [x] **Dead keyMap/help infrastructure (keys.go)** - `keyMap` struct and `defaultKeyMap()` define bindings with help text, but bindings are never used for key dispatch and `help.Model` is never rendered. ~133 lines of dead code.

- [x] **getCmdWithCancel / getNextCmdWithCancel duplication (snmp_ops.go)** - Lines 50-84 and 95-129 are structurally identical, differing only in which GoSNMP method is called. Should be a single generic function parameterized by the operation.

## High

- [x] **Consolidate treeMode branching in resultModel** - 9 methods in result_pane.go have identical `if r.treeMode { ... } else { ... }` branching. Extract an `activeTopPane()` accessor or use an interface to eliminate the dispatch.

- [x] **Entity-cascade pattern repeated 4+ times** - The Object/Notification/Group/Compliance/Capability type-switch cascade appears in `cel_filter.go:buildActivation`, `helpers.go:nodeDescription`, `style.go:nodeStatus`, and others. Should be consolidated.

- [x] **tableWalkCmdWithCancel is a 140-line god function (snmp_ops.go:234-377)** - Mixes SNMP walking, OID parsing, table schema lookup, result building, and cancellation. Should be decomposed.

- [x] **handleMouseClick is a god function (app_update.go:929-1042)** - 113 lines of nested conditionals dispatching clicks to different UI regions.

- [x] **Top-pane rendering repetition (app_draw.go)** - The pattern `styles.Pane.Width(l.rightTop.Dx()).Height(l.rightTop.Dy()).Render(...)` appears 6 times with only the inner content varying. Extract a helper.

- [x] **renderFilterIndicator duplication (app_draw.go:285-310)** - `renderFilterIndicator` and `renderResultFilterIndicator` are nearly identical.

## Medium

- [x] **Replace sort package with slices package** - `sort.Slice` -> `slices.SortFunc` in module.go:34, typebrowser.go:29. `sort.Strings` -> `slices.Sort` in query_bar.go:57, filter.go:21.

- [x] **Manual reversal instead of slices.Reverse (detail_render.go:443-463)**

- [x] **Two different truncate functions** - module.go:185-193 and table_data.go:275-291 implement truncation differently. (Inline truncation in result_pane.go extracted to truncateValue helper; the two file-level functions serve different purposes and were left as-is.)

- [ ] **Inconsistent scrolling/navigation interfaces** - Three different approaches: `navigablePane` interface, manual cursor/offset tracking (tableDataModel), and viewport-based scrolling. tableDataModel still reimplements cursor/offset/ensureVisible that ListView provides. searchModel was migrated to ListView. Remaining: tableDataModel has multi-column rendering that ListView doesn't directly support.

- [x] **Inconsistent activate/deactivate patterns** - Some panes use method calls, others use field assignments. No unified lifecycle. `queryBarModel.activate()` is the only one that doesn't call Focus internally.

- [ ] **model struct has 25+ fields (app.go)** - Several are derived state that could be computed on demand rather than stored. Observation - not actionable without larger redesign.

- [x] **Context menu value-closure pattern (context_menu.go)** - 6 action closures each repeat `r := m.results.selectedResult(); if r == nil { return m, nil }`.

- [ ] **moduleModel / typeModel are structurally parallel** - Both embed `expandableList` with identical activate/deactivate/applyFilter/view patterns. Could share a base or use generics. Deferred - would add generic complexity for modest dedup gain.

- [x] **opSession type created but never stored (snmp_ops.go)** - Dead abstraction, constructed but its reference is never retained.

- [x] **Stale "Phase N" comments (app.go:112, 115)**

- [x] **wrapText / wrapValue near-identical (detail_render.go / detail_dev.go)**

- [x] **scrollTopPaneUp/Down/By share structure** - Up/Down are just special cases of By with n=1 and n=-1. Remove the two wrappers.

- [x] **handleGetResult / handleGetNextResult share setup/teardown** - Both create a resultGroup, iterate results, set bottomPane/focus/layout identically. Extract `handleSNMPResult`.

- [x] **snmpWalk / startQueryWalk share most logic** - Both call startWalkCmd, create a resultGroup, set walkStatus, set bottomPane/focus. Extract common `startWalk` method.

- [x] **Duplicate status setting patterns** - `setStatusReturn` helper vs direct `m.status.current` assignment. Walk code bypasses the helper.

- [x] **Click-to-deactivate cascade (app_update.go:974-1005)** - 7-case switch that deactivates whichever pane was focused. Three cases (diag, module, types) are identical. Extract `deactivateCurrentFocus()`.

- [x] **resolveChord view-switching cases repeat activate/updateLayout** - Three cases (vd, vm, vy) follow identical 5-line pattern. Could be table-driven.

- [x] **Popup position clamping repeated** - tooltip.go:74-85 and context_menu.go:119-131 both clamp x/y/w/h to screen bounds. Extract `clampRect`.

## Low

- [x] **stdlib swaps (minor)** - `fmt.Sprintf("%d")` per OID arc -> `strconv.FormatUint` (result_tree.go:186-191). `fmt.Sprintf("%s")` -> direct string (snmp_format.go:44). `slices.IndexFunc` in profile.go, device_dialog.go, select.go. `slices.Clone` in snmp_ops.go.

- [x] **Unused width parameter in statusModel.view (status.go:50)**

- [x] **Hardcoded color `#2D2C35` in chord.go** - should use `palette.BgLighter`

- [x] **Inconsistent padRight vs padRightBg (tree.go:343 vs 353)** - Both still used, serve different purposes. No change needed.

- [x] **Dual clipboard approach** - both termenv and atotto/clipboard imported. Intentional: OSC 52 works over SSH, system clipboard works locally.

- [x] **normalizeVersion only applied to profiles, not CLI flags**

- [x] **resultHistory described as ring buffer but isn't one**

- [x] **requireSelectedOID returns 5 values (helpers.go)** - could use a struct

- [x] **Inconsistent setSize signatures** - some use `width, height`, others `w, h`, filterBarModel has no setSize at all

- [x] **Inline magic numbers in rendering** - `+4`, `12`, `3`, `%-10s` repeated without named constants in result_pane.go and viewFlat

- [x] **"not connected" check repeated in 5 places** - add `isConnected()` method to snmpSession

- [x] **searchModel manual scroll/cursor** - reimplements what ListView provides

- [x] **Inconsistent string formatting in detail_render.go** - mixes writeLine, fmt.Fprintf, b.WriteString within same functions
