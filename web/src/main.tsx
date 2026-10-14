import './content-security'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { RouterProvider } from '@tanstack/react-router'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { router } from './router'
import '@fontsource-variable/dm-sans'
import '@fontsource/dm-mono/400.css'
import './styles/index.css'

const element = document.getElementById('root')
if (!element) throw new Error('Application root is missing')
const queryClient = new QueryClient()
createRoot(element).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  </StrictMode>,
)
