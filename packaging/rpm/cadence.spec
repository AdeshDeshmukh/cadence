Name:           cadence
Version:        1.0.0
Release:        1%{?dist}
Summary:        Local AI-first voice dictation CLI and developer engine
Group:          Applications/Productivity
License:        Apache-2.0
URL:            https://github.com/AdeshDeshmukh/cadence
Source0:        https://github.com/AdeshDeshmukh/cadence/archive/v%{version}.tar.gz
BuildRequires:  golang >= 1.23

%description
Cadence is a local, high-performance, private voice dictation and developer engine.
Provides on-device Whisper ASR, intelligent editing, system control, and MCP integration.

%prep
%setup -q

%build
go build -ldflags="-s -w" -o bin/cadence ./cmd/cadence
go build -ldflags="-s -w" -o bin/cadenced ./cmd/cadenced

%install
rm -rf %{buildroot}
install -d %{buildroot}%{_bindir}
install -d %{buildroot}%{_userunitdir}
install -m 0755 bin/cadence %{buildroot}%{_bindir}/cadence
install -m 0755 bin/cadenced %{buildroot}%{_bindir}/cadenced
install -m 0644 packaging/deb/systemd/cadenced.service %{buildroot}%{_userunitdir}/cadenced.service

%files
%{_bindir}/cadence
%{_bindir}/cadenced
%{_userunitdir}/cadenced.service

%post
if [ $1 -eq 1 ]; then
    systemctl --global daemon-reload >/dev/null 2>&1 || :
fi

%preun
if [ $1 -eq 0 ]; then
    systemctl --global stop cadenced.service >/dev/null 2>&1 || :
    systemctl --global disable cadenced.service >/dev/null 2>&1 || :
fi

%changelog
* Fri Nov 21 2026 Adesh Deshmukh <adeshkd123@gmail.com> - 1.0.0-1
- Initial production release v1.0.0
