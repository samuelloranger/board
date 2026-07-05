<script>
  let board = $state({ todo: [], in_progress: [], done: [] });
  const columns = [["todo", "To-Do"], ["in_progress", "In Progress"], ["done", "Done"]];

  async function load() {
    const r = await fetch("/api/board?project=*");
    board = await r.json();
  }
  async function move(id, status) {
    await fetch(`/api/tasks/${id}/move`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ status }),
    });
    await load();
  }
  async function add() {
    const title = prompt("Task title");
    if (!title) return;
    await fetch("/api/tasks", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ title, status: "todo" }),
    });
    await load();
  }
  function onDrop(e, status) {
    const id = e.dataTransfer.getData("id");
    if (id) move(id, status);
  }
  $effect(() => { load(); });
</script>

<header><h1>board</h1><button onclick={add}>+ Task</button></header>
<div class="cols">
  {#each columns as [key, label]}
    <div class="col" ondragover={(e) => e.preventDefault()} ondrop={(e) => onDrop(e, key)}>
      <h2>{label} ({board[key]?.length ?? 0})</h2>
      {#each board[key] ?? [] as t}
        <div class="card" draggable="true" ondragstart={(e) => e.dataTransfer.setData("id", t.id)}>
          {t.title}
        </div>
      {/each}
    </div>
  {/each}
</div>

<style>
  header { display: flex; gap: 1rem; align-items: center; padding: 1rem; font-family: system-ui; }
  .cols { display: grid; grid-template-columns: repeat(3, 1fr); gap: 1rem; padding: 1rem; }
  .col { background: #f4f4f5; border-radius: 8px; padding: 0.5rem; min-height: 300px; }
  .card { background: white; border-radius: 6px; padding: 0.6rem; margin: 0.4rem 0; box-shadow: 0 1px 2px rgba(0,0,0,.1); cursor: grab; }
  h2 { font-size: 0.9rem; font-family: system-ui; }
</style>
