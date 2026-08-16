// Package dashboard provides the administration dashboard as assets embedded in
// the Momobase binary.
//
// The assets are a Vite build of the React application in this directory. Nothing
// under dist is committed, so the embed sits behind the "dashboard" build tag: a
// default build carries no assets and reports Available as false, while the
// release, container, and web CI builds run the bundle first and compile with
// -tags dashboard.
package dashboard
