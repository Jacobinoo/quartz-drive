import { SearchBar } from "@/components/search-bar";
import { Separator } from "@/components/ui/separator";
import { SidebarTrigger } from "@/components/ui/sidebar";
import { UserMenu } from "@/components/user-menu";

export function AppHeader() {
  return (
    <header className="flex h-16 shrink-0 items-center gap-2 justify-between border-b px-4 bg-background">
      <div className="flex items-center gap-4 flex-1">
        <SidebarTrigger className="-ml-1 md:hidden" />
        <SearchBar />
      </div>
      <div className="flex items-center gap-4">
        <Separator orientation="vertical" className="h-6" />
        <UserMenu />
      </div>
    </header>
  );
}
