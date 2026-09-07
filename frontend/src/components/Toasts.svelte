<script lang="ts">
  import { toasts, dismiss } from '../lib/toast'
</script>

<div class="toasts">
  {#each $toasts as t (t.id)}
    <div class="toast {t.kind}" role="status" onclick={() => dismiss(t.id)}>
      {t.text}
    </div>
  {/each}
</div>

<style>
  .toasts {
    position: fixed;
    right: 16px;
    bottom: 16px;
    display: flex;
    flex-direction: column;
    gap: 8px;
    z-index: 60;
    max-width: 420px;
  }
  .toast {
    padding: 10px 14px;
    border-radius: 8px;
    background: var(--panel-2);
    border: 1px solid var(--line);
    box-shadow: 0 6px 18px rgba(0, 0, 0, 0.35);
    cursor: pointer;
    font-size: 13px;
    overflow-wrap: anywhere;
    animation: rise 0.15s ease-out;
  }
  .toast.error { border-color: var(--fav); color: #ffd9d9; }
  .toast.ok { border-color: var(--ok); color: #d6f2e0; }
  @keyframes rise { from { opacity: 0; transform: translateY(6px); } }
</style>
