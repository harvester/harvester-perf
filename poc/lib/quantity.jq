# Kubernetes quantity helpers shared by the harvester-perf suites.

# "500m" / "2" / "1500000n" -> millicores
def cpu_millicores:
  if . == null then 0
  elif type == "number" then . * 1000
  elif . == "" then 0
  elif endswith("m") then (rtrimstr("m") | tonumber)
  elif endswith("u") then ((rtrimstr("u") | tonumber) / 1000)
  elif endswith("n") then ((rtrimstr("n") | tonumber) / 1000000)
  else (tonumber * 1000)
  end;

# "512Mi" / "2Gi" / "1000000" -> bytes
def mem_bytes:
  if . == null then 0
  elif type == "number" then .
  elif . == "" then 0
  else . as $s
    | [["Ki",1024],["Mi",1048576],["Gi",1073741824],["Ti",1099511627776],["Pi",1125899906842624],
       ["k",1000],["M",1000000],["G",1000000000],["T",1000000000000],["P",1000000000000000]]
    | map(select($s | endswith(.[0])))
    | if length > 0 then (.[0] as $u | ($s | rtrimstr($u[0]) | tonumber) * $u[1])
      else ($s | tonumber) end
  end;

def round2: if . == null then null else (. * 100 | round) / 100 end;
def round1: if . == null then null else (. * 10 | round) / 10 end;

def human_bytes:
  if . == null then "n/a"
  elif . >= 1099511627776 then "\((. / 1099511627776) | round2) Ti"
  elif . >= 1073741824 then "\((. / 1073741824) | round2) Gi"
  elif . >= 1048576 then "\((. / 1048576) | round2) Mi"
  elif . >= 1024 then "\((. / 1024) | round2) Ki"
  else "\(.) B" end;

# millicores -> "1.5 cores"
def human_cores:
  if . == null then "n/a" else "\((. / 1000) | round2)" end;

# percentage of $b represented by $a, one decimal, null when $b is 0/absent
def pct($a; $b): if (($b // 0) == 0) then null else (($a / $b) * 100 | round1) end;

def pct_str($a; $b): (pct($a; $b)) as $p | if $p == null then "n/a" else "\($p)%" end;

def dash: if . == null or . == "" then "-" else . end;
