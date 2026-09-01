/** Detail-panel state previously living as loose $state vars in App.svelte. */
export const detailUI = $state({
  detail: null,
  editing: false,
  edit: { title: "", description: "", priority: "", due_date: "", tags: "", project: "" },
  noteBody: "",
  answerDraft: "",
  seenNoteIds: new Set(),
  animNoteIds: {},
  runFiles: [],
});

/** Run-prompt state (path prompt + errors) owned beside the detail panel. */
export const runUI = $state({
  needPath: null,
  pathInput: "",
  runError: "",
});

export function resetDetailUI() {
  detailUI.detail = null;
  detailUI.editing = false;
  detailUI.edit = { title: "", description: "", priority: "", due_date: "", tags: "", project: "" };
  detailUI.noteBody = "";
  detailUI.answerDraft = "";
  detailUI.seenNoteIds = new Set();
  detailUI.animNoteIds = {};
  detailUI.runFiles = [];
  runUI.needPath = null;
  runUI.pathInput = "";
  runUI.runError = "";
}
