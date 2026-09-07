<script lang="ts">
  let { onAdd }: { onAdd: () => void } = $props()
</script>

<div class="welcome">
  <div class="logo">📚</div>
  <h1>Welcome to ComicGo</h1>
  <p>Point ComicGo at a folder of comic archives (.zip / .cbz).<br />
    Cover images are loaded lazily and metadata is read from each archive’s comment.</p>
  <button onclick={onAdd}>Choose comics folder…</button>
</div>

<style>
  .welcome {
    height: calc(100vh - 52px);
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 10px;
    text-align: center;
    padding: 0 20px;
  }
  .logo { font-size: 56px; }
  h1 { margin: 0; }
  p { color: var(--text-dim); line-height: 1.6; }
  button { padding: 10px 20px; font-size: 15px; }
</style>
