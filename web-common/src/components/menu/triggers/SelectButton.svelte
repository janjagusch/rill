<script lang="ts">
  import { getContext, hasContext } from "svelte";
  import type { Writable } from "svelte/store";
  import CaretDownIcon from "../../icons/CaretDownIcon.svelte";

  export let block = false;
  // FIXME: 99% sure that `disabled` is only ever
  // false and can be simplified out of the codebase
  export let disabled = false;
  export let tailwindClasses = "";
  export let activeTailwindClasses = "";
  export let active = false;
  export let level: undefined | "error" = undefined;
  // used as aria-label
  export let label: undefined | string = undefined;

  let childRequestedTooltipSuppression;
  let parentHasTooltipSomewhere = hasContext(
    "rill:app:childRequestedTooltipSuppression"
  );
  //
  if (parentHasTooltipSomewhere) {
    childRequestedTooltipSuppression = getContext(
      "rill:app:childRequestedTooltipSuppression"
    ) as Writable<boolean>;
  }

  $: if (parentHasTooltipSomewhere && active) {
    childRequestedTooltipSuppression?.set(true);
  } else {
    childRequestedTooltipSuppression?.set(false);
  }

  $: classes = {
    error: `text-red-600 hover:bg-red-200 ${
      active ? "bg-red-200" : "bg-red-100"
    }`,
    undefined: `bg-transparent ${active ? "bg-gray-200" : "bg-transparent"} ${
      disabled
        ? "ui-copy-disabled-faint italic"
        : "hover:bg-gray-200 hover:dark:bg-gray-600"
    }`,
  };
</script>

<button
  aria-label={label}
  class="
{block ? 'flex w-full h-full px-2' : 'inline-flex w-max rounded px-1'} 
  items-center gap-x-1 justify-between
  {level ? classes[level] : ''}
  {tailwindClasses}
  {active && !disabled ? activeTailwindClasses : ''}

  "
  on:click
  {disabled}
>
  <slot />
  <span
    class:hidden={disabled}
    class="{active ? '-rotate-180' : ''} transition-transform"
  >
    <CaretDownIcon />
  </span>
</button>
