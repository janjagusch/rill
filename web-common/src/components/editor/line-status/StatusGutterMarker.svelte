<script lang="ts">
  import Tooltip from "@rilldata/web-common/components/tooltip/Tooltip.svelte";
  import TooltipContent from "@rilldata/web-common/components/tooltip/TooltipContent.svelte";
  import CancelCircle from "../../icons/CancelCircle.svelte";
  import InfoCircle from "../../icons/InfoCircle.svelte";
  import WarningIcon from "../../icons/WarningIcon.svelte";

  export let level: "error" | "warning" | "info" | undefined = undefined;
  export let message: string | undefined = undefined;
  export let active = false;
</script>

<Tooltip distance={8} suppress={message === undefined}>
  <div
    class="grid justify-between text-gray-500"
    class:bg-red-50={level === "error" && !active}
    class:bg-red-100={level === "error" && active}
    class:text-red-600={level === "error" && !active}
    class:text-red-700={level === "error" && active}
    class:bg-yellow-200={level === "warning" && !active}
    class:bg-yellow-300={level === "warning" && active}
    class:text-yellow-500={level === "warning" && !active}
    class:text-yellow-600={level === "warning" && active}
    class:bg-blue-200={level === "info"}
    class:bg-blue-300={level === "info" && !active}
    class:bg-blue-500={level === "info" && active}
    class:text-blue-600={level === "info" && !active}
  >
    <div
      style:grid-column="icon"
      style:height="17px"
      style:width="24px"
      class="grid justify-center items-center"
    >
      {#if level === "error"}
        <CancelCircle />
      {:else if level === "warning"}
        <WarningIcon />
      {:else if level === "info"}
        <InfoCircle />
      {/if}
    </div>
  </div>

  <TooltipContent maxWidth="240px" slot="tooltip-content"
    >{message}</TooltipContent
  >
</Tooltip>
