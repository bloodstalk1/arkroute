# Arkroute Panel UX Refresh Design

Date: 2026-06-10
Status: Ready for user spec review

## Goal

Refresh the Arkroute local panel so setup is easier for first-time users while the panel still works as a practical control plane for configured providers, routes, CLI activation, logs, and config safety.

The panel should guide users through a clear provider setup path, then become a quiet operational dashboard once providers exist. Users must be able to add and edit providers from the panel without switching to manual YAML editing for common changes.

## Direction

Use a dark "quiet control plane" visual system:

- Off-black/tinted charcoal base instead of pure black.
- Green as the primary operational signal for ready, saved, active, and selected states.
- Blue only for secondary information such as fetch/model-discovery hints.
- Amber for warnings and red for blocking errors.
- Restrained borders, tighter 8px radii, and tinted shadows.
- A slightly sharper industrial-terminal treatment only in the Traces view.

Keep the existing stack:

- React in `web-ui/src/App.jsx`.
- CSS in `web-ui/src/index.css`.
- Existing Phosphor icon dependency.
- No new UI framework or styling library.

## Current Problems

The current panel exposes useful functionality, but the user flow is hard to follow:

- Provider setup, provider detail, route/model topology, and CLI activation compete for attention.
- The provider form is always visible, even after configuration exists, which makes the Providers tab feel like setup instead of a dashboard.
- Existing providers are visible but not editable through the same clear setup path.
- Advanced mapping fields sit close to first-run setup choices and make the first step feel more technical than necessary.
- Several component groups use inline styles, which makes polish and consistency harder.
- Some CSS uses undefined tokens such as `--border` and `--surface-soft`, causing inconsistent surfaces.
- Mobile navigation and long forms need a clearer stacked flow.
- Empty, loading, error, and post-save states are present in places but not composed as a coherent user journey.

## Primary Workflow

The Providers tab becomes the main setup and provider management surface.

### Empty State

When no provider exists:

- Show a focused empty state instead of a full dashboard.
- Primary action: `Add first provider`.
- Explain the minimum path in plain language: choose a provider, provide a key source, pick a model, save, then activate a CLI.
- Keep metrics hidden or visually secondary until there is real data.

### Configured State

When one or more providers exist:

- Show a dashboard header with `Add provider`.
- Show compact metrics for providers, routes, and models.
- Show provider cards/rows with clear actions:
  - `Edit`
  - `Test` or equivalent readiness action when supported by existing endpoints
  - route/model context links
- Show route/model overview as secondary context, not as the first thing the user must understand.

### Add/Edit Drawer

`Add provider` and `Edit` open the same drawer/sheet component.

Desktop behavior:

- Drawer slides in from the right.
- Dashboard remains visible behind it.
- Header states whether the user is adding or editing.

Mobile behavior:

- Drawer becomes a full-screen sheet.
- Back/Cancel is available at the top.
- Primary action is fixed at the bottom.

Edit mode:

- Prefill existing provider data.
- Use `Save changes` as the primary action.
- Provide `Cancel`.
- Keep destructive remove behavior separate from normal save.
- Provider removal requires inline confirmation, not `window.alert()`.

## Setup/Edit Steps

The drawer uses four short steps:

1. Provider preset
2. Key source
3. Model and route
4. Activate CLI

Users can move through the steps, but the first implementation can render them in one scrollable drawer with a visible progress row if that is simpler than building multi-page wizard state.

### Provider Preset

Show provider presets as selectable rows/cards with:

- Provider name.
- Protocol/type.
- Default model if known.
- Base URL preview.

The selected preset fills provider name, protocol, base URL, recommended env var, default upstream model, exposed alias, and route alias.

### Key Source

Show key source as a segmented choice:

- Environment variable
- Config value

Environment variable mode:

- Show the exact export command preview, for example `export OPENROUTER_API_KEY=...`.
- Validate that env name is not empty.

Config value mode:

- Show a password input.
- Never echo the stored key in provider cards or summary rows.

### Model And Route

Keep the happy path simple:

- Upstream model select.
- Exposed alias.
- Route alias.

Live model discovery:

- Automatically fetch when provider/base URL/key source is sufficient.
- Manual `Fetch live` remains available.
- On failure, show inline status and keep curated model options available.
- Do not block setup solely because live discovery failed when curated or custom model entry is available.

Advanced mapping:

- Hide provider name, protocol, base URL, and advanced alias controls behind disclosure when they are not needed for the preset happy path.
- Preserve full control for custom providers.

### Activate CLI

After saving:

- Show a confirmation state inside the drawer.
- Offer next actions:
  - Copy CLI environment command.
  - Launch supported CLI when backend says launch is available.
  - Close.
- If activation fails, show the error inline with remediation text returned by the backend.

## Tab Design

### Providers

Providers is the default task surface:

- Empty state for first setup.
- Configured dashboard after setup.
- Add/Edit drawer.
- Provider readiness/detail context.
- CLI setup context for the selected provider/model where available.

### Models & Routes

Models & Routes becomes a topology dashboard:

- Left side: route list and registered model list.
- Right side: selected detail with CLI context and policy inspector.
- Route presets appear as an action panel, not mixed into the primary list.
- Selecting a route/model updates CLI context and policy inspector without forcing a tab switch.

### Traces

Traces keeps the most terminal-like visual treatment:

- Dense log stream.
- Fixed-width timing.
- Severity/event labels with restrained color.
- Clear empty state when no events exist.
- Preserve current polling behavior unless backend adds pause/clear support.

### CLI Tools

CLI Tools becomes a readiness console:

- Show binary, gateway, and model discovery status in a scan-friendly layout.
- Make `Launch` the primary action only when launch is supported.
- Keep `Copy Env` available as the reliable fallback.
- Show launch-blocked reason inline.

### System

System separates runtime status from config safety:

- Gateway status card with provider/model/route counts.
- Config safety card for export, redacted export, copy redacted, validate import, and apply import.
- Config import textarea should not dominate the entire page.
- Resources remain secondary.

## Component System

Define shared CSS patterns instead of one-off inline styling:

- Buttons: primary, secondary, tertiary, danger.
- Icon buttons for close, copy, launch, edit, remove, refresh.
- Status badges: ok, pending, info, warning, error.
- Inline status messages for field-level and section-level feedback.
- Empty states with consistent icon, title, copy, and optional action.
- Drawer/sheet layout.
- Metric tiles.
- List rows.
- Code/command blocks.
- Form fields with helper text and validation text.

Remove inline styles from `App.jsx` where they only express spacing, colors, or layout.

## Accessibility And Interaction

- Preserve visible focus rings for all interactive elements.
- Do not rely on color alone for error, warning, or selected states.
- Use semantic buttons for actions.
- Keep labels associated with form fields.
- Ensure long model IDs, URLs, env names, and commands wrap or scroll without breaking layout.
- Add pressed/disabled states for buttons.
- Replace ambiguous status copy with direct messages.
- Avoid `window.alert()`; use inline confirmation and status panels.

## Responsive Behavior

Desktop:

- Sidebar remains persistent.
- Providers dashboard uses a constrained content width and drawer from the right.
- Two-column layouts are allowed for detail workbenches.

Tablet:

- Sidebar can collapse or compress.
- Main workbenches collapse to one column before content becomes cramped.

Mobile:

- Navigation becomes compact top navigation with short labels where space allows.
- Drawer becomes full-screen sheet.
- Primary actions span the width where needed.
- Metrics stack.
- Provider rows and route/model rows use single-column content.

## Data And Backend Impact

The refresh should avoid new backend requirements where possible.

Expected frontend-only changes:

- Visual tokens and layout.
- Drawer state.
- Add/edit form mode.
- Inline validation.
- Status and empty-state composition.
- Reuse existing setup, status, model fetch, route preset, CLI context, CLI tools, config transfer, and policy override endpoints.

Possible backend follow-up if not already supported:

- Editing an existing provider through the setup endpoint may need stable update semantics.
- Removing a provider safely may need a dedicated endpoint if current config import/apply is the only mutation path.
- Provider readiness testing may need an endpoint if `fetch-models` is not suitable as a lightweight check.

The implementation plan should inspect existing panel endpoints before adding backend routes.

## Verification

Required checks for implementation:

- `npm run build --prefix web-ui`
- Focused frontend tests if existing test setup supports the changed helper functions.
- Go tests for any backend endpoint changes.
- Manual browser check at desktop and mobile widths.

Known current condition:

- `npm run lint --prefix web-ui` currently fails on existing React hook lint rules and warnings. The implementation plan should either address those lint failures as part of the refresh or explicitly keep lint out of the completion gate for this slice.

## Acceptance Criteria

- A first-time user can start from an empty Providers tab and find `Add first provider` immediately.
- Adding a provider is guided by preset, key source, model/route, and CLI activation steps.
- Existing providers can be edited from the panel.
- The configured Providers tab reads as a dashboard, not a permanent setup form.
- Advanced mapping remains available but does not dominate the happy path.
- Save, error, loading, live model discovery, and post-save states are inline and clear.
- Mobile setup remains usable through a full-screen sheet.
- Undefined CSS tokens and layout-affecting inline styles are removed.
- Traces retains a distinct terminal feel while matching the refreshed visual system.
- No upstream provider API key is exposed in summaries, cards, logs, or generic errors.
