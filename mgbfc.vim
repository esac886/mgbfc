if exists('current_compiler')
  finish
endif
let current_compiler = 'mgbfc'

let s:save_cpo = &cpo
set cpo-=C

CompilerSet makeprg=mgbfc
CompilerSet errorformat=%f:%l:%c:\ %m

let &cpo = s:save_cpo
unlet s:save_cpo
