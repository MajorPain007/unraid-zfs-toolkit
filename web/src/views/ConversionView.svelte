<script lang="ts">
  let { opts = {} }: { opts?: Record<string, any> } = $props();
</script>

<div class="zt-empty">
  <h3>Conversion</h3>
  <p>{JSON.stringify(opts)}</p>
</div>
