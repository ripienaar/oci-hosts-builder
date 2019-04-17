%define debug_package %{nil}
%define pkgname {{cpkg_name}}
%define version {{cpkg_version}}
%define release {{cpkg_release}}
%define dist {{cpkg_dist}}
%define binary {{cpkg_binary}}
%define tarball {{cpkg_tarball}}

Name: %{pkgname}
Version: %{version}
Release: %{release}.%{dist}
Summary: Oracle Cloud /etc/hosts builder
License: Apache-2.0
URL: https://devco.net
Group: System Tools
Packager: R.I.Pienaar <rip@devco.net>
Source0: %{tarball}
BuildRoot: %{_tmppath}/%{pkgname}-%{version}-%{release}-root-%(%{__id_u} -n)
Requires(pre): shadow-utils

%description
Creates /etc/hosts based on all Private IPs found in a compartment or set of compartments

%prep
%setup -q

%build

%pre
getent group ocihostsmgr >/dev/null || groupadd -r ocihostsmgr
getent passwd ocihostsmgr >/dev/null || \
    useradd -r -g ocihostsmgr -d /home/ocihostsmgr -s /bin/bash -m -c "/etc/hosts manager" ocihostsmgr
exit 0

%install
rm -rf %{buildroot}
%{__install} -d -m0755  %{buildroot}/usr/bin
%{__install} -m0755 %{binary} %{buildroot}/usr/bin/%{pkgname}

%clean
rm -rf %{buildroot}

%files
/usr/bin/%{pkgname}

%changelog
* Tue Aug 07 2018 R.I.Pienaar <rip@devco.net>
- Initial Release