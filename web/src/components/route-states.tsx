import { PageHeading } from './page'

export function RouteError() {
  return (
    <div className="page-enter">
      <PageHeading
        title="This view could not load"
        description="Refresh the page to retry. Other fleet views remain available from the navigation."
      />
    </div>
  )
}

export function RouteNotFound() {
  return (
    <div className="page-enter">
      <PageHeading
        title="Page not found"
        description="Use the workspace navigation to return to your fleet."
      />
    </div>
  )
}
