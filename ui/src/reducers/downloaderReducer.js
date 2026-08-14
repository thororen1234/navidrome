import {
  EVENT_DOWNLOAD_STATUS,
  EVENT_TOOL_INSTALL_STATUS,
} from '../actions'

const initialState = {
  // Latest known status per job id, keyed by id. The queue table merges this over its
  // fetched list so it can update live without polling.
  jobs: {},
  // Latest install/upgrade/repair status per tool, keyed by tool name.
  tools: {},
}

export const downloaderReducer = (previousState = initialState, payload) => {
  const { type, data } = payload
  switch (type) {
    case EVENT_DOWNLOAD_STATUS:
      return { ...previousState, jobs: { ...previousState.jobs, [data.id]: data } }
    case EVENT_TOOL_INSTALL_STATUS:
      return { ...previousState, tools: { ...previousState.tools, [data.tool]: data } }
    default:
      return previousState
  }
}
