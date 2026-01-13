"use client"

import * as React from "react"
import { Check, ChevronsUpDown, Loader2 } from "lucide-react"

import { cn } from "@/lib/utils"
import { Button } from "@/components/ui/button"
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import { Badge } from "@/components/ui/badge"
import { listProviderModels, listUserProviderModels, ProviderModel } from "@/lib/api"

interface ModelSelectorProps {
  provider: string
  value: string
  onValueChange: (value: string) => void
  disabled?: boolean
  placeholder?: string
  /** Use user endpoint instead of admin endpoint (default: false for backward compatibility) */
  useUserEndpoint?: boolean
}

export function ModelSelector({
  provider,
  value,
  onValueChange,
  disabled = false,
  placeholder = "Type or search for a model...",
  useUserEndpoint = false,
}: ModelSelectorProps) {
  const [open, setOpen] = React.useState(false)
  const [models, setModels] = React.useState<ProviderModel[]>([])
  const [loading, setLoading] = React.useState(false)
  const [error, setError] = React.useState<string | null>(null)
  const [inputValue, setInputValue] = React.useState(value)
  const selectionMadeRef = React.useRef(false)

  // Sync input with external value
  React.useEffect(() => {
    setInputValue(value)
  }, [value])

  // Load models when provider changes or popover opens
  React.useEffect(() => {
    if (!provider || !open) return

    const loadModels = async () => {
      setLoading(true)
      setError(null)
      try {
        const fetchFn = useUserEndpoint ? listUserProviderModels : listProviderModels
        const result = await fetchFn(provider)
        setModels(result.models)
      } catch (err) {
        setError(err instanceof Error ? err.message : "Failed to load models")
        setModels([])
      } finally {
        setLoading(false)
      }
    }

    loadModels()
  }, [provider, open, useUserEndpoint])

  // Filter models based on search
  const filteredModels = React.useMemo(() => {
    if (!inputValue) return models

    const search = inputValue.toLowerCase()

    // Special filters
    if (search === ":free" || search === "free") {
      return models.filter(m => m.is_free)
    }

    // Regular search - check id, name, and description
    return models.filter(m =>
      m.id.toLowerCase().includes(search) ||
      m.name.toLowerCase().includes(search) ||
      (m.description && m.description.toLowerCase().includes(search))
    )
  }, [models, inputValue])

  // Group models: free models first, then paid
  const groupedModels = React.useMemo(() => {
    const freeModels = filteredModels.filter(m => m.is_free)
    const paidModels = filteredModels.filter(m => !m.is_free)
    return { freeModels, paidModels }
  }, [filteredModels])

  const handleInputChange = (newValue: string) => {
    setInputValue(newValue)
  }

  const handleInputKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter") {
      e.preventDefault()
      // Commit the current input value
      onValueChange(inputValue)
      setOpen(false)
    }
    if (e.key === "Escape") {
      setOpen(false)
    }
  }

  const handleBlur = () => {
    // Commit changes when losing focus (after a short delay to allow item selection)
    setTimeout(() => {
      // Don't overwrite if a selection was just made
      if (selectionMadeRef.current) {
        selectionMadeRef.current = false
        return
      }
      if (inputValue !== value) {
        onValueChange(inputValue)
      }
    }, 200)
  }

  const selectModel = (modelId: string) => {
    selectionMadeRef.current = true
    setInputValue(modelId)
    onValueChange(modelId)
    setOpen(false)
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          role="combobox"
          aria-expanded={open}
          className="w-full justify-between font-normal"
          disabled={disabled || !provider}
        >
          <span className="truncate">
            {value || placeholder}
          </span>
          <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-[400px] p-0" align="start">
        <Command shouldFilter={false}>
          <CommandInput
            placeholder="Type model ID or search... (:free for free models)"
            value={inputValue}
            onValueChange={handleInputChange}
            onKeyDown={handleInputKeyDown}
            onBlur={handleBlur}
          />
          <CommandList>
            {loading && (
              <div className="flex items-center justify-center py-6">
                <Loader2 className="h-4 w-4 animate-spin mr-2" />
                <span className="text-sm text-muted-foreground">Loading models...</span>
              </div>
            )}
            {error && (
              <div className="py-6 text-center text-sm text-destructive">
                {error}
              </div>
            )}

            {/* Show typed value as an option if it doesn't match any model */}
            {!loading && inputValue && !models.some(m => m.id === inputValue) && (
              <CommandGroup heading="Custom Model">
                <CommandItem
                  value={inputValue}
                  onSelect={() => selectModel(inputValue)}
                >
                  <Check
                    className={cn(
                      "mr-2 h-4 w-4",
                      value === inputValue ? "opacity-100" : "opacity-0"
                    )}
                  />
                  <div className="flex flex-col flex-1 min-w-0">
                    <span className="truncate font-medium">{inputValue}</span>
                    <span className="text-xs text-muted-foreground">
                      Press Enter to use this model ID
                    </span>
                  </div>
                </CommandItem>
              </CommandGroup>
            )}

            {!loading && !error && filteredModels.length === 0 && !inputValue && (
              <CommandEmpty>Type a model ID or search...</CommandEmpty>
            )}

            {!loading && !error && groupedModels.freeModels.length > 0 && (
              <CommandGroup heading="Free Models">
                {groupedModels.freeModels.map((model) => (
                  <CommandItem
                    key={model.id}
                    value={model.id}
                    onSelect={() => selectModel(model.id)}
                  >
                    <Check
                      className={cn(
                        "mr-2 h-4 w-4",
                        value === model.id ? "opacity-100" : "opacity-0"
                      )}
                    />
                    <div className="flex flex-col flex-1 min-w-0">
                      <div className="flex items-center gap-2">
                        <span className="truncate font-medium">{model.name}</span>
                        <Badge variant="secondary" className="text-xs shrink-0">
                          FREE
                        </Badge>
                      </div>
                      <span className="text-xs text-muted-foreground truncate">
                        {model.id}
                      </span>
                    </div>
                    {model.context_size && (
                      <span className="text-xs text-muted-foreground shrink-0 ml-2">
                        {(model.context_size / 1000).toFixed(0)}k ctx
                      </span>
                    )}
                  </CommandItem>
                ))}
              </CommandGroup>
            )}
            {!loading && !error && groupedModels.paidModels.length > 0 && (
              <CommandGroup heading={groupedModels.freeModels.length > 0 ? "Paid Models" : "Models"}>
                {groupedModels.paidModels.map((model) => (
                  <CommandItem
                    key={model.id}
                    value={model.id}
                    onSelect={() => selectModel(model.id)}
                  >
                    <Check
                      className={cn(
                        "mr-2 h-4 w-4",
                        value === model.id ? "opacity-100" : "opacity-0"
                      )}
                    />
                    <div className="flex flex-col flex-1 min-w-0">
                      <span className="truncate font-medium">{model.name}</span>
                      <span className="text-xs text-muted-foreground truncate">
                        {model.id}
                      </span>
                    </div>
                    {model.context_size && (
                      <span className="text-xs text-muted-foreground shrink-0 ml-2">
                        {(model.context_size / 1000).toFixed(0)}k ctx
                      </span>
                    )}
                  </CommandItem>
                ))}
              </CommandGroup>
            )}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}
